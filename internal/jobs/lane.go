package jobs

import (
	"context"
	"sort"
	"time"

	"github.com/azamasad/pingpong/internal/store"
)

func laneRank(lane string) int {
	if lane == store.LaneBatch {
		return 1
	}
	return 0
}

func (s *Service) waitTurn(ctx context.Context, id, machine string) error {
	limit := 1
	if m, ok := s.cfg.MachineByName(machine); ok && m.MaxConcurrent > 0 {
		limit = m.MaxConcurrent
	}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		s.holdMu.Lock()
		held := s.holding[machine]
		admit := false
		if held < limit {
			waiting, err := s.store.WaitingForMachine(ctx, machine)
			if err != nil {
				s.holdMu.Unlock()
				return err
			}
			sort.SliceStable(waiting, func(i, j int) bool {
				ri, rj := laneRank(waiting[i].Lane), laneRank(waiting[j].Lane)
				if ri != rj {
					return ri < rj
				}
				return waiting[i].CreatedAt.Before(waiting[j].CreatedAt)
			})
			slots := limit - held
			for i := 0; i < len(waiting) && i < slots; i++ {
				if waiting[i].ID == id {
					admit = true
					break
				}
			}
		}
		if admit {
			s.holding[machine]++
			s.holdMu.Unlock()
			return nil
		}
		s.holdMu.Unlock()
		timer := s.clk.NewTimer(20 * time.Millisecond)
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (s *Service) releaseTurn(machine string) {
	s.holdMu.Lock()
	if s.holding[machine] > 0 {
		s.holding[machine]--
	}
	s.holdMu.Unlock()
}
