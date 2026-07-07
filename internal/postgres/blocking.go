package postgres

import "math"

// ResolveBlockedBy sets each snapshot's BlockedByPID to its single most-direct
// blocker: among all PIDs blocking it, the one highest in the blocking tree
// (shortest path to a root), or 0 when nothing blocks it.
// Example: (-> means blocked by) C -> B -> A, D -> C, D -> B. Result: D is blocked by B.
func ResolveBlockedBy(snapshots []ActivitySnapshot) {
	blockers := make(map[int32][]int32, len(snapshots))
	for _, s := range snapshots {
		blockers[s.PID] = cleanBlockers(s.PID, s.BlockingPids)
	}

	depth := blockerDepths(blockers)

	for i := range snapshots {
		snapshots[i].BlockedByPID = pickBlocker(blockers[snapshots[i].PID], depth)
	}
}

func cleanBlockers(pid int32, raw []int32) []int32 {
	if len(raw) == 0 {
		return nil
	}

	seen := make(map[int32]struct{}, len(raw))
	out := make([]int32, 0, len(raw))
	for _, b := range raw {
		if b <= 0 || b == pid {
			continue
		}
		if _, dup := seen[b]; dup {
			continue
		}
		seen[b] = struct{}{}
		out = append(out, b)
	}

	return out
}

// blockerDepths returns each PID's shortest path length to a root via multi-source
// BFS over the reversed edges. PIDs trapped in a cycle never reach a root and are
// absent from the result.
func blockerDepths(blockers map[int32][]int32) map[int32]int {
	// blocker -> waiters it blocks
	blocks := make(map[int32][]int32)
	nodes := make(map[int32]struct{})
	for waiter, bs := range blockers {
		nodes[waiter] = struct{}{}
		for _, b := range bs {
			nodes[b] = struct{}{}
			blocks[b] = append(blocks[b], waiter)
		}
	}

	depth := make(map[int32]int, len(nodes))
	queue := make([]int32, 0, len(nodes))
	for n := range nodes {
		// nothing blocks n -> it is a root
		if len(blockers[n]) == 0 {
			depth[n] = 0
			queue = append(queue, n)
		}
	}

	// BFS visits nodes in non-decreasing depth, so the first time a waiter
	// is reached is via its shortest path to a root.
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, waiter := range blocks[cur] {
			// already visited -> shortest path already recorded
			if _, done := depth[waiter]; done {
				continue
			}
			depth[waiter] = depth[cur] + 1
			queue = append(queue, waiter)
		}
	}

	return depth
}

// pickBlocker returns the lowest-depth blocker, ties broken by lowest PID.
func pickBlocker(blockers []int32, depth map[int32]int) int32 {
	best := int32(0)
	bestDepth := 0
	for _, b := range blockers {
		d, ok := depth[b]
		if !ok {
			d = math.MaxInt
		}
		if best == 0 || d < bestDepth || (d == bestDepth && b < best) {
			best = b
			bestDepth = d
		}
	}

	return best
}
