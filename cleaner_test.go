package legalhold

import (
	"slices"
	"testing"
)

func assertPurge(t *testing.T, c *Cleaner, now int64, want []int64) {
	t.Helper()
	got := c.Purge(now)
	if got == nil {
		got = []int64{}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("Purge(%d) = %v, want %v", now, got, want)
	}
}

// 相同区间的两项保全逐个解除：解除一项后消息仍受保护，
// 最后一项解除后已到期消息立即清退，未到期消息继续等待。
func TestIdenticalHoldsReleasedOneByOne(t *testing.T) {
	c := NewCleaner([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 500},
		{ID: 2, SentAt: 200, ExpireAt: 900},
	})

	c.AddHold(10, 0, 300)
	c.AddHold(11, 0, 300) // 区间完全相同，仍是独立保全

	// 两项保全都在，消息 1 已到期也不能清退。
	assertPurge(t, c, 600, []int64{})

	// 解除其中一项，另一项仍然保护两条消息。
	c.RemoveHold(10)
	assertPurge(t, c, 600, []int64{})

	// 最后一项解除：已到期的消息 1 立即清退，未到期的消息 2 等待。
	c.RemoveHold(11)
	assertPurge(t, c, 600, []int64{1})

	// 清退是最终决定：消息 1 不会再次返回。
	assertPurge(t, c, 600, []int64{})

	// 消息 2 到期后即可清退。
	assertPurge(t, c, 900, []int64{2})
}

// 多个区间交叠后只释放局部消息；候选消息发送顺序与到期顺序相反；
// 检查区间端点（含起点、不含终点）与到期等于当前时刻的清退。
func TestOverlappingHoldsPartialRelease(t *testing.T) {
	c := NewCleaner([]Message{
		// 发送时刻递增，到期时刻递减：先发的不先到期。
		{ID: 1, SentAt: 100, ExpireAt: 1000},
		{ID: 2, SentAt: 200, ExpireAt: 800},
		{ID: 3, SentAt: 300, ExpireAt: 600},
		{ID: 4, SentAt: 400, ExpireAt: 400},
	})

	c.AddHold(20, 100, 300) // 覆盖消息 1、2（含起点 100，不含终点 300）
	c.AddHold(21, 200, 400) // 覆盖消息 2、3

	// 到期时刻等于当前时刻即可清退：消息 4（ExpireAt=400）不受保全，立即清退。
	assertPurge(t, c, 400, []int64{4})

	// 解除 [100,300)：消息 1 失去保护但未到期；消息 2 仍被 [200,400) 保护。
	c.RemoveHold(20)
	assertPurge(t, c, 1000, []int64{1})

	// 解除 [200,400)：消息 2、3 均已到期且不再受保护，一并清退。
	c.RemoveHold(21)
	assertPurge(t, c, 1000, []int64{2, 3})
}

// 区间端点语义：起点包含、终点不包含。
func TestHoldRangeEndpoints(t *testing.T) {
	c := NewCleaner([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 100}, // 恰好在起点
		{ID: 2, SentAt: 200, ExpireAt: 200}, // 恰好在终点
	})
	c.AddHold(30, 100, 200)

	// 起点消息受保护，终点消息不受保护。
	assertPurge(t, c, 1000, []int64{2})

	c.RemoveHold(30)
	assertPurge(t, c, 1000, []int64{1})
}

// 新增保全不追溯已清退的消息。
func TestHoldDoesNotRevivePurged(t *testing.T) {
	c := NewCleaner([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 100},
	})
	assertPurge(t, c, 100, []int64{1})

	// 清退后新增覆盖该消息的保全，也不能让它重新出现。
	c.AddHold(40, 0, 1000)
	assertPurge(t, c, 1000, []int64{})
	c.RemoveHold(40)
	assertPurge(t, c, 1000, []int64{})
}

// 解除不存在的保全不影响现有保护。
func TestRemoveMissingHold(t *testing.T) {
	c := NewCleaner([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 100},
	})
	c.AddHold(50, 0, 200)
	c.RemoveHold(999)
	assertPurge(t, c, 1000, []int64{})

	c.RemoveHold(50)
	assertPurge(t, c, 1000, []int64{1})
}
