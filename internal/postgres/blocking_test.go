package postgres_test

import (
	"testing"

	"github.com/querysheriff/collector/internal/postgres"
)

func snap(pid int32, blockers ...int32) postgres.ActivitySnapshot {
	return postgres.ActivitySnapshot{PID: pid, BlockingPids: blockers}
}

func TestResolveBlockedBy(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		snapshots []postgres.ActivitySnapshot
		want      map[int32]int32 // pid -> expected BlockedByPID
	}{
		{
			name:      "no blockers leaves zero",
			snapshots: []postgres.ActivitySnapshot{snap(10), snap(20)},
			want:      map[int32]int32{10: 0, 20: 0},
		},
		{
			name:      "simple chain picks the direct blocker",
			snapshots: []postgres.ActivitySnapshot{snap(10), snap(20, 10)},
			want:      map[int32]int32{10: 0, 20: 10},
		},
		{
			// 20 waits on 10, 30 on 20, 40 on both 20 and 30: 40 gets 20, one step from the root, not two.
			name:      "diamond picks blocker highest in the tree",
			snapshots: []postgres.ActivitySnapshot{snap(10), snap(20, 10), snap(30, 20), snap(40, 20, 30)},
			want:      map[int32]int32{10: 0, 20: 10, 30: 20, 40: 20},
		},
		{
			// Both blockers sit at depth 1, so the lower PID wins.
			name:      "equal depth ties break on lowest pid",
			snapshots: []postgres.ActivitySnapshot{snap(10), snap(20, 10), snap(30, 10), snap(40, 30, 20)},
			want:      map[int32]int32{10: 0, 20: 10, 30: 10, 40: 20},
		},
		{
			// 99 is never sampled, so it counts as a root and a direct holder wins.
			name:      "unsampled blocker counts as a root",
			snapshots: []postgres.ActivitySnapshot{snap(20, 99), snap(30, 20)},
			want:      map[int32]int32{20: 99, 30: 20},
		},
		{
			name:      "two-way deadlock stays deterministic",
			snapshots: []postgres.ActivitySnapshot{snap(10, 20), snap(20, 10)},
			want:      map[int32]int32{10: 20, 20: 10},
		},
		{
			name:      "three-way deadlock stays deterministic",
			snapshots: []postgres.ActivitySnapshot{snap(10, 20), snap(20, 30), snap(30, 10)},
			want:      map[int32]int32{10: 20, 20: 30, 30: 10},
		},
		{
			// 20 and 30 wait on each other, so neither reaches a root: 40 gets 10 instead.
			name:      "rooted blocker beats cyclic blocker",
			snapshots: []postgres.ActivitySnapshot{snap(5), snap(10, 5), snap(20, 30), snap(30, 20), snap(40, 10, 20)},
			want:      map[int32]int32{5: 0, 10: 5, 20: 30, 30: 20, 40: 10},
		},
		{
			name:      "self and duplicate blockers are ignored",
			snapshots: []postgres.ActivitySnapshot{snap(10), snap(20, 20, 10, 10, 0)},
			want:      map[int32]int32{10: 0, 20: 10},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			snapshots := tt.snapshots
			postgres.ResolveBlockedBy(snapshots)
			for _, s := range snapshots {
				if s.BlockedByPID != tt.want[s.PID] {
					t.Errorf("pid %d: BlockedByPID = %d, want %d", s.PID, s.BlockedByPID, tt.want[s.PID])
				}
			}
		})
	}
}
