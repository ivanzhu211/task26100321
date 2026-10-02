# legalpurge

法律保全感知的进程内消息清退库（Go），由上层清理任务直接调用。

模块只管理候选消息的元数据（编号、发送时刻、最早可永久删除时刻），
不保存正文或附件，也不执行文件删除；`Purge` 返回的编号即最终删除决定，
由调用方执行实际删除。

## 概念

- 每个 `Purger` 实例处理一个邮箱的一轮清理，初始化时传入固定候选集，
  期间不新增或修改候选消息。时间统一使用 UTC 整数秒。
- 保全（hold）有独立编号，覆盖发送时刻处于 `[start, end)` 的尚存候选消息
  （含起点、不含终点）。区间完全相同的不同编号属于相互独立的保全。
- 消息被任意一项有效保全覆盖即不可清退；解除一项只撤销该项保护，
  最后一项保护解除后，已到期消息立即重新具备清退资格。
- 清退是最终决定：同一消息不会再次返回，之后新增保全不追溯已清退消息。
- 调用按顺序串行执行，传入的当前时刻应单调不减。`Purger` 非并发安全。

## API

```go
type Message struct {
    ID       int64 // 唯一整数编号
    SentAt   int64 // 发送时刻，UTC 秒
    ExpireAt int64 // 最早可永久删除时刻，UTC 秒，与发送时刻不保证同序
}

func NewPurger(candidates []Message) *Purger
func (p *Purger) AddHold(holdID, start, end int64) // 覆盖 [start, end)
func (p *Purger) RemoveHold(holdID int64)          // 不存在的编号为无操作
func (p *Purger) Purge(now int64) []int64          // 返回并移除已到期且未受保全的编号（升序）
```

到期时刻等于当前时刻（`ExpireAt <= now`）即可清退。

## 示例

```go
p := legalpurge.NewPurger([]legalpurge.Message{
    {ID: 1, SentAt: 100, ExpireAt: 500},
    {ID: 2, SentAt: 200, ExpireAt: 900},
})

p.AddHold(10, 0, 300)   // 案件 A 保全 [0, 300)
p.AddHold(11, 0, 300)   // 案件 B 保全同一区间，独立编号
p.Purge(600)            // => []，两项保全均有效

p.RemoveHold(10)        // 解除案件 A
p.Purge(600)            // => []，案件 B 仍保护

p.RemoveHold(11)        // 解除案件 B
p.Purge(600)            // => [1]，1 已到期；2 未到期继续等待
p.Purge(900)            // => [2]
```

## 测试

```sh
go test ./...
```

覆盖场景：相同区间两项保全逐个解除、交叠区间局部释放
（发送顺序与到期顺序相反）、区间端点语义、到期等于当前时刻、
保全不追溯已清退消息、解除未知编号为无操作。
