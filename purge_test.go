package legalpurge

import (
	"reflect"
	"testing"
)

// 场景一：相同区间的两项保全逐个解除。
// 解除其中一项后消息仍受保护，最后一项解除后按到期情况清退。
func TestIdenticalHoldsReleasedOneByOne(t *testing.T) {
	p := NewPurger([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 500}, // 已到期
		{ID: 2, SentAt: 200, ExpireAt: 900}, // 未到期
	})

	p.AddHold(10, 0, 300)
	p.AddHold(11, 0, 300) // 与 10 完全相同的独立保全

	if got := p.Purge(600); len(got) != 0 {
		t.Fatalf("两项保全有效时不应清退任何消息, got %v", got)
	}

	p.RemoveHold(10)
	if got := p.Purge(600); len(got) != 0 {
		t.Fatalf("解除一项后另一项仍应保护消息, got %v", got)
	}

	p.RemoveHold(11)
	// 最后一项保护解除：已到期的 1 立即清退，未到期的 2 继续等待。
	if got := p.Purge(600); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("最后一项解除后应只清退已到期消息, got %v", got)
	}
	if got := p.Purge(600); len(got) != 0 {
		t.Fatalf("同一消息不能再次返回, got %v", got)
	}
	if got := p.Purge(900); !reflect.DeepEqual(got, []int64{2}) {
		t.Fatalf("到期时刻等于当前时刻即可清退, got %v", got)
	}
}

// 场景二：多个区间交叠后只释放局部消息。
// 候选消息发送顺序与到期顺序相反，检查区间端点与到期相等时刻。
func TestOverlappingHoldsPartialRelease(t *testing.T) {
	p := NewPurger([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 1000}, // 发送最早，到期最晚
		{ID: 2, SentAt: 200, ExpireAt: 900},
		{ID: 3, SentAt: 300, ExpireAt: 800}, // 发送最晚，到期最早
	})

	p.AddHold(20, 100, 300) // 覆盖 1、2（含起点 100，不含终点 300）
	p.AddHold(21, 200, 400) // 覆盖 2、3

	if got := p.Purge(1000); len(got) != 0 {
		t.Fatalf("交叠保全下不应清退任何消息, got %v", got)
	}

	// 解除 20 后：1 不再受任何保护，2 仍被 21 覆盖，3 仍被 21 覆盖。
	p.RemoveHold(20)
	if got := p.Purge(1000); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("只应释放局部消息 1, got %v", got)
	}

	// 解除 21 后：2、3 均失去保护，按各自到期时刻清退。
	p.RemoveHold(21)
	if got := p.Purge(899); !reflect.DeepEqual(got, []int64{3}) {
		t.Fatalf("899 时仅 3 到期, got %v", got)
	}
	if got := p.Purge(900); !reflect.DeepEqual(got, []int64{2}) {
		t.Fatalf("900 时 2 到期（等于当前时刻即可清退）, got %v", got)
	}
}

// 区间端点语义：含起点、不含终点。
func TestHoldIntervalEndpoints(t *testing.T) {
	p := NewPurger([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 500}, // 恰好位于起点
		{ID: 2, SentAt: 300, ExpireAt: 500}, // 恰好位于终点
	})
	p.AddHold(30, 100, 300)

	if got := p.Purge(500); !reflect.DeepEqual(got, []int64{2}) {
		t.Fatalf("起点应受保护、终点不受保护, got %v", got)
	}
	p.RemoveHold(30)
	if got := p.Purge(500); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("解除后 1 应可清退, got %v", got)
	}
}

// 保全不追溯已清退的消息。
func TestHoldDoesNotApplyRetroactively(t *testing.T) {
	p := NewPurger([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 500},
	})
	if got := p.Purge(500); !reflect.DeepEqual(got, []int64{1}) {
		t.Fatalf("无保全时应清退, got %v", got)
	}
	p.AddHold(40, 0, 1000)
	if got := p.Purge(1000); len(got) != 0 {
		t.Fatalf("新增保全不应追溯已清退消息, got %v", got)
	}
}

// 解除不存在的保全编号是无操作。
func TestRemoveUnknownHoldIsNoop(t *testing.T) {
	p := NewPurger([]Message{
		{ID: 1, SentAt: 100, ExpireAt: 500},
	})
	p.AddHold(50, 0, 200)
	p.RemoveHold(999)
	if got := p.Purge(500); len(got) != 0 {
		t.Fatalf("解除未知编号不应影响现有保全, got %v", got)
	}
}
