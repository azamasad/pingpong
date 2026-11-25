package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/azamasad/pingpong/internal/apperrors"
	"github.com/azamasad/pingpong/internal/jobs"
)

const fileUploadExtraBytes = 64 << 10

func (s *Server) handleBatchFromFile(w http.ResponseWriter, r *http.Request) {
	if !s.jobsEnabled(w) {
		return
	}
	maxBody := int64(s.Cfg.Files.MaxBytes) + fileUploadExtraBytes
	if cl := r.ContentLength; cl > maxBody && cl > 0 {
		writeErr(w, apperrors.New("FILE_TOO_LARGE", "upload too large"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody+1)
	defer r.Body.Close()
	mp, err := r.MultipartReader()
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "multipart required"))
		return
	}
	fields := map[string]string{}
	var fileName string
	var fileBytes []byte
	for {
		part, err := mp.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			if err.Error() == "http: request body too large" {
				writeErr(w, apperrors.New("FILE_TOO_LARGE", "upload too large"))
				return
			}
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid multipart"))
			return
		}
		name := part.FormName()
		data, err := io.ReadAll(io.LimitReader(part, maxBody+1))
		if err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid multipart"))
			return
		}
		if int64(len(data)) > maxBody {
			writeErr(w, apperrors.New("FILE_TOO_LARGE", "upload too large"))
			return
		}
		if name == "file" {
			fileName = part.FileName()
			fileBytes = data
		} else {
			fields[name] = string(data)
		}
	}
	if len(fileBytes) == 0 {
		writeErr(w, apperrors.New("BAD_REQUEST", "file required"))
		return
	}
	text, errMsg, errCode := validateTextFile(fileName, fileBytes)
	if errCode != "" {
		writeErr(w, apperrors.New(errCode, errMsg))
		return
	}
	chunkChars := s.Cfg.Files.ChunkChars
	if v := fields["chunk_chars"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 500 || n > 20000 {
			writeErr(w, apperrors.New("BAD_REQUEST", "chunk_chars out of range"))
			return
		}
		chunkChars = n
	}
	dryRun := fields["dry_run"] == "1"
	instruction := strings.TrimSpace(fields["instruction"])
	tplName := strings.TrimSpace(fields["template"])
	if instruction == "" && tplName == "" {
		writeErr(w, apperrors.New("BAD_REQUEST", "instruction or template required"))
		return
	}
	if instruction != "" && tplName != "" {
		writeErr(w, apperrors.New("BAD_REQUEST", "use instruction or template, not both"))
		return
	}
	vars, err := parseFormVars(fields["vars"])
	if err != nil {
		writeErr(w, apperrors.New("BAD_REQUEST", "invalid vars"))
		return
	}
	var plan *jobs.SplitPlan
	if raw := strings.TrimSpace(fields["split"]); raw != "" {
		plan = &jobs.SplitPlan{}
		if err := json.Unmarshal([]byte(raw), plan); err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid split"))
			return
		}
	}
	var reduce *jobs.ReduceSpec
	if raw := strings.TrimSpace(fields["reduce"]); raw != "" {
		reduce = &jobs.ReduceSpec{}
		if err := json.Unmarshal([]byte(raw), reduce); err != nil {
			writeErr(w, apperrors.New("BAD_REQUEST", "invalid reduce"))
			return
		}
	}
	routing := fileBatchRouting(fields)
	if dryRun {
		chunks, chars := s.Jobs.ChunkCount(text, chunkChars)
		if chunks == 0 {
			writeErr(w, apperrors.New("BAD_REQUEST", "no text after chunking"))
			return
		}
		if chunks > s.Jobs.MaxBatchItems() {
			writeErr(w, apperrors.New("BAD_REQUEST", "too many chunks, raise chunk_chars"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"chunks": chunks, "chars": chars})
		return
	}
	res, err := s.Jobs.SubmitFileBatch(r.Context(), jobs.FileBatchInput{
		FileText: text, Instruction: instruction, Template: tplName,
		Vars: vars, ChunkChars: chunkChars, Split: plan, Reduce: reduce, Routing: routing,
	})
	if err != nil {
		if writeJobErr(w, err) {
			return
		}
		msg := "could not create batch"
		if re, ok := err.(*jobs.RouteRejectedError); ok && re.Code == "BAD_REQUEST" {
			if n, _ := s.Jobs.ChunkCount(text, chunkChars); n > s.Jobs.MaxBatchItems() {
				msg = "too many chunks, raise chunk_chars"
			}
		}
		writeErr(w, apperrors.New("BAD_REQUEST", msg))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"id": res.ID, "job_ids": res.JobIDs, "chunks": res.Chunks, "reduce_job_id": nil,
	})
}

func parseFormVars(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return map[string]string{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return nil, err
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		s, ok := v.(string)
		if !ok {
			return nil, errors.New("bad vars")
		}
		out[k] = s
	}
	return out, nil
}

func fileBatchRouting(fields map[string]string) map[string]any {
	out := map[string]any{}
	if v := fields["machine"]; v != "" {
		out["machine"] = v
	}
	if v := fields["model"]; v != "" {
		out["model"] = v
	}
	if v := fields["profile"]; v != "" {
		out["profile"] = v
	}
	if fields["fast"] == "1" || strings.EqualFold(fields["fast"], "true") {
		out["fast"] = true
	}
	return out
}

func validateTextFile(name string, data []byte) (text, msg, code string) {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".txt", ".md", ".markdown":
	case ".pdf":
		return "", "PDF is not supported yet", "FILE_TYPE_UNSUPPORTED"
	default:
		return "", "unsupported file type", "FILE_TYPE_UNSUPPORTED"
	}
	if bytes.Contains(data, []byte{0}) {
		return "", "file contains NUL bytes", "FILE_TYPE_UNSUPPORTED"
	}
	if !utf8.Valid(data) {
		return "", "file is not valid UTF-8", "FILE_TYPE_UNSUPPORTED"
	}
	text = strings.TrimSpace(string(data))
	if text == "" {
		return "", "empty file", "BAD_REQUEST"
	}
	return text, "", ""
}
