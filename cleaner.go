// Package legalhold 提供法律保全感知的消息清退能力。
//
// 每个 Cleaner 实例处理一个邮箱的一轮清理：初始化时传入固定候选集，
// 之后支持新增保全、解除保全与按当前时刻清退。数据只保存在进程内，
// 模块只管理候选消息元数据，不保存正文或附件，也不执行文件删除。
package legalhold

import (
	"slices"
	"sort"
)

// Message 是一条候选消息的元数据。时间统一使用 UTC 整数秒。
type Message struct {
	ID       int64 // 唯一整数编号
	SentAt   int64 // 发送时刻
	ExpireAt int64 // 最早可永久删除时刻（到期时刻）
}

// Cleaner 管理一个邮箱一轮清理的候选消息与保全集合。
type Cleaner struct {
	msgs map[int64]Message // 尚存候选消息，键为消息编号

	holds    map[int64]holdRange // 有效保全，键为保全编号
	bounds   []boundEvent        // 由保全区间生成的扫描线事件（惰性重建）
	coverage []int               // 与 bounds 对齐的前缀覆盖计数
	dirty    bool                // 保全变更后标记，查询前重建
}

// holdRange 表示一项保全的覆盖区间 [Start, End)，包含起点、不包含终点。
type holdRange struct {
	Start int64
	End   int64
}

// boundEvent 是扫描线事件：在 At 时刻覆盖计数变化 Delta。
type boundEvent struct {
	At    int64
	Delta int
}

// NewCleaner 以固定候选集创建实例。期间不新增或修改候选消息。
func NewCleaner(candidates []Message) *Cleaner {
	c := &Cleaner{
		msgs:  make(map[int64]Message, len(candidates)),
		holds: make(map[int64]holdRange),
	}
	for _, m := range candidates {
		c.msgs[m.ID] = m
	}
	return c
}

// AddHold 新增一项编号为 id 的保全，覆盖发送时刻处于 [start, end)
// 的所有尚存候选消息。即使区间与其他保全完全相同，也是独立保全。
// 若 id 已存在，则以新区间替换旧区间。
func (c *Cleaner) AddHold(id int64, start, end int64) {
	c.holds[id] = holdRange{Start: start, End: end}
	c.dirty = true
}

// RemoveHold 解除编号为 id 的保全，只撤销这一项的保护。
// 解除不存在的保全不产生任何效果。
func (c *Cleaner) RemoveHold(id int64) {
	if _, ok := c.holds[id]; ok {
		delete(c.holds, id)
		c.dirty = true
	}
}

// Purge 返回并移除已到期（ExpireAt <= now）且未受任何有效保全覆盖的
// 消息编号。清退结果是最终删除决定，同一消息不会再次返回。
// 返回的编号按升序排列。
func (c *Cleaner) Purge(now int64) []int64 {
	c.rebuild()
	var purged []int64
	for id, m := range c.msgs {
		if m.ExpireAt <= now && !c.covered(m.SentAt) {
			purged = append(purged, id)
		}
	}
	for _, id := range purged {
		delete(c.msgs, id)
	}
	slices.Sort(purged)
	return purged
}

// rebuild 在保全集合变更后、首次查询前重建扫描线事件与前缀覆盖计数。
func (c *Cleaner) rebuild() {
	if !c.dirty {
		return
	}
	events := make([]boundEvent, 0, 2*len(c.holds))
	for _, h := range c.holds {
		if h.Start >= h.End {
			continue // 空区间不覆盖任何消息
		}
		events = append(events, boundEvent{At: h.Start, Delta: 1})
		events = append(events, boundEvent{At: h.End, Delta: -1})
	}
	slices.SortFunc(events, func(a, b boundEvent) int {
		switch {
		case a.At < b.At:
			return -1
		case a.At > b.At:
			return 1
		default:
			return 0
		}
	})
	// 合并同一时刻的事件并计算前缀覆盖计数。
	c.bounds = c.bounds[:0]
	c.coverage = c.coverage[:0]
	for _, e := range events {
		if n := len(c.bounds); n > 0 && c.bounds[n-1].At == e.At {
			c.bounds[n-1].Delta += e.Delta
			c.coverage[n-1] += e.Delta
		} else {
			c.bounds = append(c.bounds, e)
			if n > 0 {
				e.Delta += c.coverage[n-1]
			}
			c.coverage = append(c.coverage, e.Delta)
		}
	}
	c.dirty = false
}

// covered 报告发送时刻 t 是否被任意一项有效保全覆盖，
// 即满足 Start <= t < End 的保全数量大于零。
func (c *Cleaner) covered(t int64) bool {
	// 找到最后一个 At <= t 的事件，其前缀计数即为 t 时刻的覆盖数。
	i := sort.Search(len(c.bounds), func(i int) bool {
		return c.bounds[i].At > t
	})
	return i > 0 && c.coverage[i-1] > 0
}
