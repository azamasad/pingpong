package jobs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/azamasad/pingpong/internal/filechunk"
	"github.com/azamasad/pingpong/internal/logx"
	"github.com/azamasad/pingpong/internal/route"
	"github.com/azamasad/pingpong/internal/store"
	"github.com/azamasad/pingpong/internal/templates"
	"github.com/google/uuid"
)

// FileBatchInput is everything needed to create a chunked file batch.
type FileBatchInput struct {
	FileText    string
	Instruction string
	Template    string
	Vars        map[string]string
	ChunkChars  int
	Split       *SplitPlan
	Reduce      *ReduceSpec
	Routing     map[string]any
}

// FileBatchSubmitResult is the API outcome of from-file.
type FileBatchSubmitResult struct {
	ID          string
	JobIDs      []string
	Chunks      int
	ReduceJobID *string
}

func (s *Service) ChunkCount(text string, chunkChars int) (chunks int, chars int) {
	ch := filechunk.SplitParagraphs(text, chunkChars)
	return len(ch), len([]rune(text))
}

func (s *Service) SubmitFileBatch(ctx context.Context, in FileBatchInput) (*FileBatchSubmitResult, error) {
	chunks := filechunk.SplitParagraphs(in.FileText, in.ChunkChars)
	if len(chunks) == 0 {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	max := s.MaxBatchItems()
	if len(chunks) > max {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	plan := in.Split
	if plan != nil {
		if err := validateSplitTargets(s, plan); err != nil {
			return nil, err
		}
	}
	tplName := in.Template
	var tpl *store.Template
	if tplName != "" {
		var err error
		tpl, err = s.loadTemplate(ctx, tplName)
		if err != nil {
			return nil, err
		}
		if !strings.Contains(tpl.Body, "{{chunk}}") {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
	} else if !strings.Contains(in.Instruction, "{{chunk}}") {
		return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
	}
	var reduceJSON string
	if in.Reduce != nil {
		if _, ok := s.cfg.MachineByName(in.Reduce.Machine); !ok {
			return nil, &RouteRejectedError{Code: route.CodeMachineUnknown}
		}
		model := in.Reduce.Model
		model, _, _ = s.cfg.ResolveAlias(model, in.Reduce.Machine)
		res := s.resolve(route.Request{Model: model, Machine: in.Reduce.Machine})
		if !res.OK && !retryableCode(res.Code) {
			return nil, &RouteRejectedError{Code: res.Code}
		}
		if !strings.Contains(in.Reduce.Instruction, "{{results}}") {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		b, _ := json.Marshal(in.Reduce)
		reduceJSON = string(b)
	}
	batchID := uuid.NewString()
	now := s.clk.Now().UTC()
	kind := "file"
	var planJSON string
	if plan != nil {
		b, _ := json.Marshal(plan)
		planJSON = string(b)
	}
	total := len(chunks)
	jobs := make([]*store.Job, 0, total)
	splitIdx := 0
	for i, chunk := range chunks {
		vars := map[string]string{
			"chunk": chunk,
			"index": fmt.Sprintf("%d", i+1),
			"total": fmt.Sprintf("%d", total),
		}
		for k, v := range in.Vars {
			vars[k] = v
		}
		var bodyJSON string
		if tpl != nil {
			body, _, err := s.expandOneTemplate(tpl, tplName, vars, in.Routing)
			if err != nil {
				return nil, err
			}
			bodyJSON = body
		} else {
			prompt, missing, _ := templates.Substitute(in.Instruction, vars)
			if len(missing) > 0 {
				return nil, &TemplateRejectError{Code: "TEMPLATE_VARS_MISSING", Missing: missing}
			}
			req := map[string]any{"prompt": prompt, "lane": store.LaneBatch}
			for k, v := range in.Routing {
				req[k] = v
			}
			b, err := json.Marshal(req)
			if err != nil {
				return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
			}
			bodyJSON = string(b)
		}
		cb, _, err := parseChatBody(bodyJSON)
		if err != nil {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		if itemLaneInvalid(cb) {
			return nil, &RouteRejectedError{Code: "BAD_REQUEST"}
		}
		pinM, pinMod := "", ""
		if plan != nil && !itemSelfRouted(cb) {
			ti := splitTargetIndex(splitIdx, *plan)
			splitIdx++
			pinM = plan.Targets[ti].Machine
			pinMod = plan.Targets[ti].Model
		}
		j, err := s.prepareBatchJob(bodyJSON, batchID, pinM, pinMod)
		if err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	batch := &store.Batch{
		ID: batchID, CreatedAt: now, Kind: kind,
		ItemCount: total, PlanJSON: planJSON, Source: tplName,
		ReduceJSON: reduceJSON,
	}
	err := s.store.WithTx(ctx, func(tx *sql.Tx) error {
		if err := s.store.InsertBatchTx(ctx, tx, batch); err != nil {
			return err
		}
		for _, j := range jobs {
			if err := s.store.InsertJobTx(ctx, tx, j); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
		go s.runLoop(j.ID)
	}
	logx.Event("info", "batch_submitted", map[string]any{
		"batch_id": batchID, "items": total, "source": "file",
	})
	return &FileBatchSubmitResult{ID: batchID, JobIDs: ids, Chunks: total}, nil
}