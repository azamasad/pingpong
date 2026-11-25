package api

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/export"
	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/store"
)

type exportOpts struct {
	format          string
	includeThinking bool
	partial         bool
	raw             bool
	bom             bool
}

func parseExportOpts(r *http.Request) (exportOpts, error) {
	format := r.URL.Query().Get("format")
	if format != "csv" && format != "md" {
		return exportOpts{}, apperrors.New("BAD_REQUEST", "format must be csv or md")
	}
	o := exportOpts{format: format}
	o.includeThinking = r.URL.Query().Get("include_thinking") == "1"
	o.partial = r.URL.Query().Get("partial") == "1"
	o.raw = r.URL.Query().Get("raw") == "1"
	o.bom = r.URL.Query().Get("bom") == "1"
	return o, nil
}

func (s *Server) handleBatchExport(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	opts, err := parseExportOpts(r)
	if err != nil {
		writeErr(w, err.(apperrors.APIError))
		return
	}
	batchID := r.PathValue("id")
	st := s.Jobs.Store()
	hasJobs, err := st.BatchJobIDsExist(r.Context(), batchID)
	if err != nil || !hasJobs {
		if _, berr := st.GetBatch(r.Context(), batchID); berr != nil {
			writeErr(w, apperrors.New("NOT_FOUND", "batch not found"))
			return
		}
		if !hasJobs {
			writeErr(w, apperrors.New("NOT_FOUND", "batch not found"))
			return
		}
	}
	prefix := batchID
	if len(prefix) > 8 {
		prefix = prefix[:8]
	}
	name := fmt.Sprintf("pingpong-batch-%s-%s", prefix, time.Now().UTC().Format("20060102"))
	s.streamExport(w, r, name, opts, false, 0, func(ctx context.Context) (int, error) {
		idx, err := st.BatchItemIndex(ctx, batchID)
		if err != nil {
			return 0, err
		}
		rows, err := st.ExportBatchJobs(ctx, batchID)
		if err != nil {
			return 0, err
		}
		return s.writeJobRows(ctx, w, rows, idx, nil, opts)
	})
}

func (s *Server) handleJobsExport(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	opts, err := parseExportOpts(r)
	if err != nil {
		writeErr(w, err.(apperrors.APIError))
		return
	}
	rawIDs := r.URL.Query().Get("ids")
	if rawIDs == "" {
		writeErr(w, apperrors.New("BAD_REQUEST", "ids required"))
		return
	}
	ids := splitIDs(rawIDs)
	if len(ids) == 0 {
		writeErr(w, apperrors.New("BAD_REQUEST", "ids required"))
		return
	}
	if len(ids) > 200 {
		writeErr(w, apperrors.New("BAD_REQUEST", "at most 200 ids"))
		return
	}
	st := s.Jobs.Store()
	found, err := st.CountExistingJobIDs(r.Context(), ids)
	if err != nil {
		writeErr(w, apperrors.New("INTERNAL", "export failed"))
		return
	}
	skipped := len(ids) - found
	name := fmt.Sprintf("pingpong-jobs-%s", time.Now().UTC().Format("20060102"))
	s.streamExport(w, r, name, opts, true, skipped, func(ctx context.Context) (int, error) {
		rows, _, err := st.ExportJobsByIDs(ctx, ids)
		if err != nil {
			return 0, err
		}
		return s.writeJobRows(ctx, w, rows, nil, map[string]*store.JobIndex{}, opts)
	})
}

func splitIDs(s string) []string {
	parts := strings.Split(s, ",")
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func (s *Server) streamExport(w http.ResponseWriter, r *http.Request, filename string, opts exportOpts,
	jobsExport bool, skipped int, run func(context.Context) (int, error)) {
	start := time.Now()
	ctype := "text/csv; charset=utf-8"
	ext := "csv"
	if opts.format == "md" {
		ctype = "text/markdown; charset=utf-8"
		ext = "md"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.%s"`, filename, ext))
	w.Header().Set("Cache-Control", "no-store")
	if jobsExport {
		w.Header().Set("X-Pingpong-Skipped", strconv.Itoa(skipped))
	}
	if opts.bom && opts.format == "csv" {
		_, _ = w.Write([]byte{0xEF, 0xBB, 0xBF})
	}
	flusher, _ := w.(http.Flusher)
	items := 0
	var hdrErr error
	if opts.format == "csv" {
		hdrErr = export.WriteCSVHeader(w, opts.includeThinking)
	} else {
		_, hdrErr = w.Write([]byte("# Pingpong export\n\n"))
	}
	if hdrErr != nil {
		abortExport(w, flusher)
		logx.Event("warn", "export_done", map[string]any{
			"items": items, "duration_ms": time.Since(start).Milliseconds(), "outcome": "error",
		})
		return
	}
	if flusher != nil {
		flusher.Flush()
	}
	items, err := run(r.Context())
	if err != nil {
		abortExport(w, flusher)
		logx.Event("warn", "export_done", map[string]any{
			"items": items, "duration_ms": time.Since(start).Milliseconds(), "outcome": "error",
		})
		return
	}
	outcome := "ok"
	if items == 0 {
		outcome = "empty"
	}
	logx.Event("info", "export_done", map[string]any{
		"items": items, "duration_ms": time.Since(start).Milliseconds(), "outcome": outcome,
	})
}

func abortExport(w http.ResponseWriter, flusher http.Flusher) {
	if hj, ok := w.(http.Hijacker); ok {
		conn, _, err := hj.Hijack()
		if err == nil {
			_ = conn.Close()
			return
		}
	}
	if flusher != nil {
		flusher.Flush()
	}
}

func (s *Server) writeJobRows(ctx context.Context, w io.Writer, rows *sql.Rows, batchIdx *store.JobIndex,
	batchCache map[string]*store.JobIndex, opts exportOpts) (int, error) {
	defer rows.Close()
	if batchCache == nil {
		batchCache = map[string]*store.JobIndex{}
	}
	st := s.Jobs.Store()
	count := 0
	flusher, _ := w.(http.Flusher)
	for rows.Next() {
		j, err := store.ScanJobRow(rows)
		if err != nil {
			return count, err
		}
		if !opts.partial && !export.Terminal(j.Status) {
			continue
		}
		idx, hasIdx := 0, false
		if batchIdx != nil {
			if v, ok := batchIdx.ByID[j.ID]; ok {
				idx, hasIdx = v, true
			}
		} else if j.BatchID != "" {
			idx, hasIdx = st.BatchItemIndexForJob(ctx, j.BatchID, j.ID, j.ParentID, batchCache)
		}
		ot, tps, _ := st.DoneEventStats(ctx, j.ID)
		stats := &export.DoneStats{OutputTokens: ot, OutputTokensPerS: tps}
		row := export.BuildRow(j, idx, hasIdx, stats, opts.includeThinking)
		if err := writeExportRow(w, row, opts); err != nil {
			return count, err
		}
		count++
		if flusher != nil {
			flusher.Flush()
		}
	}
	if err := rows.Err(); err != nil {
		return count, err
	}
	return count, nil
}

func writeExportRow(w io.Writer, row export.Row, opts exportOpts) error {
	if opts.format == "csv" {
		return export.WriteCSVRow(w, export.RowCSVCols(row, opts.includeThinking), opts.raw)
	}
	return export.WriteMarkdownSection(w, row, opts.includeThinking)
}
