// Package legalpurge 提供法律保全感知的进程内消息清退能力。
//
// 每个 Purger 实例处理一个邮箱的一轮清理：初始化时传入固定候选集，
// 之后支持新增保全、解除保全和按当前时刻清退。模块只管理消息元数据，
// 不保存正文或附件，也不执行任何文件删除。
package legalpurge

import "sort"

// Message 描述一条候选消息的元数据，时间统一使用 UTC 整数秒。
type Message struct {
	ID       int64 // 唯一整数编号
	SentAt   int64 // 发送时刻
	ExpireAt int64 // 最早可永久删除时刻，与发送时刻不保证同序
}

// hold 表示一项有效保全，覆盖发送时刻处于 [Start, End) 的消息。
type hold struct {
	Start int64
	End   int64
}

// Purger 管理一个邮箱一轮清理的候选消息与保全。非并发安全，
// 调用方需按顺序串行调用。
type Purger struct {
	messages map[int64]Message // 尚存候选消息，清退后即移除
	holds    map[int64]hold    // 当前有效的保全
}

// NewPurger 以固定候选集初始化一个清退实例。期间不新增或修改候选消息。
func NewPurger(candidates []Message) *Purger {
	p := &Purger{
		messages: make(map[int64]Message, len(candidates)),
		holds:    make(map[int64]hold),
	}
	for _, m := range candidates {
		p.messages[m.ID] = m
	}
	return p
}

// AddHold 新增一项编号为 holdID 的保全，覆盖发送时刻处于 [start, end)
// 的尚存候选消息。区间完全相同的不同编号属于相互独立的保全。
// 保全不追溯已清退的消息。
func (p *Purger) AddHold(holdID, start, end int64) {
	p.holds[holdID] = hold{Start: start, End: end}
}

// RemoveHold 解除编号为 holdID 的保全，只撤销这一项的保护；
// 不存在的编号视为无操作。
func (p *Purger) RemoveHold(holdID int64) {
	delete(p.holds, holdID)
}

// held 报告发送时刻为 sentAt 的消息是否被任意一项有效保全覆盖。
func (p *Purger) held(sentAt int64) bool {
	for _, h := range p.holds {
		if sentAt >= h.Start && sentAt < h.End {
			return true
		}
	}
	return false
}

// Purge 返回并移除到期时刻 <= now 且未受任何有效保全覆盖的消息编号。
// 返回的编号按升序排列。清退是最终决定，同一消息不会再次返回。
func (p *Purger) Purge(now int64) []int64 {
	var purged []int64
	for id, m := range p.messages {
		if m.ExpireAt <= now && !p.held(m.SentAt) {
			purged = append(purged, id)
		}
	}
	for _, id := range purged {
		delete(p.messages, id)
	}
	sort.Slice(purged, func(i, j int) bool { return purged[i] < purged[j] })
	return purged
}
