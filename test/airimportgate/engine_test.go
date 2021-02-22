package airimportgate_test

import (
	"context"
	"testing"
	"time"
	"account-assets.kucoin.net/funding/internal/airimportgate"
)

// fixedClock 返回固定时钟。
func fixedClock(t time.Time) func() time.Time {
	return func() time.Time { return t }
}

// noopReader 不查询 airgap.Service 的 SessionReader（相位检测 bypass）。

type noopReader struct{}

func (noopReader) ReadSession(_ context.Context, _ string) (airimportgate.SessionSnapshot, error) {
	return airimportgate.SessionSnapshot{Phase: "exported", Threshold: 3, ActualQuorum: 3}, nil
}

// exportedReader 返回 exported 相位的 SessionReader。
type exportedReader struct {
	threshold	int
	actualQuorum	int
}

func (r exportedReader) ReadSession(_ context.Context, _ string) (airimportgate.SessionSnapshot, error) {
	return airimportgate.SessionSnapshot{Phase: "exported", Threshold: r.threshold, ActualQuorum: r.actualQuorum}, nil
}

// abortedReader 返回 aborted 相位的 SessionReader（相位冲突触发）。
type abortedReader struct{}

func (abortedReader) ReadSession(_ context.Context, _ string) (airimportgate.SessionSnapshot, error) {
	return airimportgate.SessionSnapshot{Phase: "aborted", Threshold: 2, ActualQuorum: 2}, nil
}

// ── BeginImport 表驱动 ─────────────────────────────────────────────────

func TestEngine_BeginImport(t *testing.T) {
	baseTime := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	validGen := airimportgate.MakeImportGen("test-uuid-0001-0000-000000000001", baseTime.Add(-10*time.Second))
	cases := []struct {
		name		string
		reader		airimportgate.SessionReader
		req		airimportgate.BeginImportReq
		wantErr		error
		wantState	airimportgate.LeaseState
	}{
		{
			name:	"正常开立租约",
			reader:	noopReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"sess-001",
				ImportGen:	validGen,
				BlobDigest:	airimportgate.DigestBlob([]byte("blob-content-001")),
				Actor:		"alice",
			},
			wantErr:	nil,
			wantState:	airimportgate.LeaseImporting,
		},
		{
			name:	"ImportGen 为空",
			reader:	noopReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"sess-002",
				ImportGen:	"",
				BlobDigest:	airimportgate.DigestBlob([]byte("blob-002")),
				Actor:		"alice",
			},
			wantErr:	airimportgate.ErrImportGenInvalid,
		},
		{
			name:	"ImportGen 已过期",
			reader:	noopReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"sess-003",
				ImportGen:	airimportgate.MakeImportGen("test-uuid-0003", baseTime.Add(-400*time.Second)),
				BlobDigest:	airimportgate.DigestBlob([]byte("blob-003")),
				Actor:		"alice",
			},
			wantErr:	airimportgate.ErrImportGenExpired,
		},
		{
			name:	"SessionID 为空",
			reader:	noopReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"",
				ImportGen:	validGen,
				BlobDigest:	airimportgate.DigestBlob([]byte("blob-004")),
				Actor:		"alice",
			},
			wantErr:	airimportgate.ErrEmptySessionID,
		},
		{
			name:	"Actor 为空",
			reader:	noopReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"sess-005",
				ImportGen:	validGen,
				BlobDigest:	airimportgate.DigestBlob([]byte("blob-005")),
				Actor:		"",
			},
			wantErr:	airimportgate.ErrInvalidActor,
		},
		{
			name:	"BlobDigest 为空",
			reader:	noopReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"sess-006",
				ImportGen:	validGen,
				BlobDigest:	"",
				Actor:		"alice",
			},
			wantErr:	airimportgate.ErrEmptyBlob,
		},
		{
			name:	"相位冲突（Session 已 aborted）",
			reader:	abortedReader{},
			req: airimportgate.BeginImportReq{
				SessionID:	"sess-007",
				ImportGen:	validGen,
				BlobDigest:	airimportgate.DigestBlob([]byte("blob-007")),
				Actor:		"bob",
			},
			wantErr:	airimportgate.ErrPhaseConflict,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
			engine := airimportgate.NewEngine(store, tc.reader, airimportgate.WithClock(fixedClock(baseTime)))
			result,
				err := engine.BeginImport(context.Background(), tc.req)
			if tc.wantErr != nil {
				if err != tc.wantErr {
					t.Errorf("want err %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if result.Lease == nil {
				t.Fatal("expected lease, got nil")
			}
			if result.Lease.State != tc.wantState {
				t.Errorf("want state %s, got %s", tc.wantState, result.Lease.State)
			}
		})
	}
}

// ── 并发导入互斥 ─────────────────────────────────────────────────────────

func TestEngine_ConcurrentImportMutex(t *testing.T) {
	baseTime := time.Date(2025,
		11, 4, 10, 0, 0, 0, time.UTC)
	gen := airimportgate.MakeImportGen("mutex-uuid-0001", baseTime.Add(-5*time.Second))
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	ctx := context.Background()
	// 第一次 BeginImport 成功
	r1, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-mutex-001",
		ImportGen:	gen,
		BlobDigest:	airimportgate.DigestBlob([]byte("blob-mutex-001")),
		Actor:		"alice",
	})
	if err != nil {
		t.Fatalf("first BeginImport should succeed, got %v", err)
	}
	if r1.Lease.State != airimportgate.LeaseImporting {
		t.Errorf("want importing, got %s", r1.Lease.State)
	}
	// 第二次 BeginImport 同 Session 应被拒绝
	gen2 := airimportgate.MakeImportGen("mutex-uuid-0002", baseTime.Add(-3*time.Second))
	_, err = engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-mutex-001",
		ImportGen:	gen2,
		BlobDigest:	airimportgate.DigestBlob([]byte("blob-mutex-002")),
		Actor:		"bob",
	})
	if err != airimportgate.ErrConcurrentImport {
		t.Errorf("want ErrConcurrentImport, got %v", err)
	}
}

// ── 密封 blob 重放拒绝 ───────────────────────────────────────────────────

func TestEngine_BlobReplayRejection(t *testing.T) {
	baseTime := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	blob := []byte("sealed-blob-content-replay-test")
	digest := airimportgate.DigestBlob(blob)
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	ctx := context.Background()
	// 预置摘要（已成功导入）
	_ = store.SaveDigest(ctx, &airimportgate.SealedBlobDigest{
		DigestID:	"digest-pre-001",
		DigestHex:	digest,
		SessionID:	"sess-replay-001",
		LeaseID:	"lease-pre-001",
		ImportedBy:	"alice",
		ImportedAt:	baseTime,
	})
	engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	gen := airimportgate.MakeImportGen("replay-uuid-0001", baseTime.Add(-5*time.Second))
	_, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-replay-002",
		ImportGen:	gen,
		BlobDigest:	digest,	// 与已导入摘要相同
		Actor:		"bob",
	})
	if err != airimportgate.ErrBlobReplay {
		t.Errorf("want ErrBlobReplay, got %v", err)
	}
}

// ── 法定人数不足拒绝 ─────────────────────────────────────────────────────

func TestEngine_QuorumDriftRejection(t *testing.T) {
	baseTime := time.Date(2025,
		11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	ctx := context.Background()
	gen := airimportgate.MakeImportGen("quorum-uuid-0001", baseTime.Add(-5*time.Second))
	r, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-quorum-001",
		ImportGen:	gen,
		BlobDigest:	airimportgate.DigestBlob([]byte("quorum-blob-001")),
		Actor:		"alice",
	})
	if err != nil {
		t.Fatalf("BeginImport failed: %v", err)
	}
	// CommitImport 时 actual_quorum < threshold
	err = engine.CommitImport(ctx, airimportgate.CommitImportReq{
		LeaseID:	r.Lease.LeaseID,
		SessionID:	"sess-quorum-001",
		ActualQuorum:	1,	// 不足
		Threshold:	3,
		BlobDigest:	r.Lease.BlobDigest,
		Actor:		"alice",
	})
	if err != airimportgate.ErrQuorumDrift {
		t.Errorf("want ErrQuorumDrift, got %v", err)
	}
	// 验证租约已被 RollbackImport 推进到 aborted
	lease, _ := engine.GetLease(ctx, r.Lease.LeaseID)
	if lease.State != airimportgate.LeaseAborted {
		t.Errorf("want state aborted after quorum drift, got %s", lease.State)
	}
}

// ── 正常完整流程（BeginImport → CommitImport） ───────────────────────────────

func TestEngine_FullImportFlow(t *testing.T) {
	baseTime := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	ctx := context.Background()
	gen := airimportgate.MakeImportGen("full-uuid-0001", baseTime.Add(-5*time.Second))
	blob := []byte("full-flow-sealed-blob")
	digest := airimportgate.DigestBlob(blob)
	// 1. BeginImport
	r, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-full-001",
		ImportGen:	gen,
		BlobDigest:	digest,
		Actor:		"alice",
	})
	if err != nil {
		t.Fatalf("BeginImport: %v", err)
	}
	if r.Lease.State != airimportgate.LeaseImporting {
		t.Errorf("want importing, got %s", r.Lease.State)
	}
	// 2. CommitImport
	err = engine.CommitImport(ctx, airimportgate.CommitImportReq{
		LeaseID:	r.Lease.LeaseID,
		SessionID:	"sess-full-001",
		ActualQuorum:	3,
		Threshold:	3,
		BlobDigest:	digest,
		Actor:		"alice",
	})
	if err != nil {
		t.Fatalf("CommitImport: %v", err)
	}
	// 3. 验证租约已 imported
	lease, err := engine.GetLease(ctx, r.Lease.LeaseID)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if lease.State != airimportgate.LeaseImported {
		t.Errorf("want imported, got %s", lease.State)
	}
	if lease.ImportedAt == nil {
		t.Error("ImportedAt should not be nil")
	}
	// 4. 验证摘要已写入
	d, err := engine.GetDigestByHex(ctx, digest)
	if err != nil {
		t.Fatalf("GetDigestByHex: %v", err)
	}
	if d.SessionID != "sess-full-001" {
		t.Errorf("digest session_id mismatch: %s", d.SessionID)
	}
	// 5. 验证法定人数绑定
	bind, err := engine.GetBindByLease(ctx, r.Lease.LeaseID)
	if err != nil {
		t.Fatalf("GetBindByLease: %v", err)
	}
	if !bind.QuorumPassed {
		t.Error("quorum_passed should be true")
	}
}

// ── RollbackImport 幂等 ──────────────────────────────────────────────────

func TestEngine_RollbackImportIdempotent(t *testing.T) {
	baseTime := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	ctx := context.Background()
	gen := airimportgate.MakeImportGen("rollback-uuid-0001", baseTime.Add(-5*time.Second))
	r, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-rollback-001",
		ImportGen:	gen,
		BlobDigest:	airimportgate.DigestBlob([]byte("rollback-blob-001")),
		Actor:		"alice",
	})
	if err != nil {
		t.Fatalf("BeginImport: %v", err)
	}
	// 第一次 Rollback
	err = engine.RollbackImport(ctx, airimportgate.RollbackImportReq{
		LeaseID:	r.Lease.LeaseID, Actor: "alice", Reason: "test rollback",
	})
	if err != nil {
		t.Fatalf("first RollbackImport: %v", err)
	}
	// 第二次 Rollback 幂等（不报错）
	err = engine.RollbackImport(ctx, airimportgate.RollbackImportReq{
		LeaseID:	r.Lease.LeaseID, Actor: "alice", Reason: "idempotent rollback",
	})
	if err != nil {
		t.Fatalf("second RollbackImport should be idempotent, got %v", err)
	}
	lease,
		_ := engine.GetLease(ctx, r.Lease.LeaseID)
	if lease.State != airimportgate.LeaseAborted {
		t.Errorf("want aborted, got %s", lease.State)
	}
}

// ── ForceAbort 双人确认 ──────────────────────────────────────────────────

func TestEngine_ForceAbort(t *testing.T) {
	baseTime := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name	string
		req	airimportgate.ForceAbortReq
		wantErr	error
	}{
		{
			name:	"正常双人确认",
			req: airimportgate.ForceAbortReq{
				ConfirmerOne:	"alice",
				ConfirmerTwo:	"bob",
				Reason:		"emergency abort",
			},
			wantErr:	nil,
		},
		{
			name:	"同一操作人拒绝",
			req: airimportgate.ForceAbortReq{
				ConfirmerOne:	"alice",
				ConfirmerTwo:	"alice",
				Reason:		"single actor",
			},
			wantErr:	airimportgate.ErrInvalidActor,
		},
		{
			name:	"确认人之一为空",
			req: airimportgate.ForceAbortReq{
				ConfirmerOne:	"alice",
				ConfirmerTwo:	"",
				Reason:		"missing actor",
			},
			wantErr:	airimportgate.ErrInvalidActor,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
			engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
			ctx := context.Background()
			gen := airimportgate.MakeImportGen("force-uuid-"+tc.name, baseTime.Add(-5*time.Second))
			r, berr := engine.BeginImport(ctx, airimportgate.BeginImportReq{
				SessionID:	"sess-force-" + tc.name,
				ImportGen:	gen,
				BlobDigest:	airimportgate.DigestBlob([]byte("force-blob-" + tc.name)),
				Actor:		"alice",
			})
			if tc.wantErr == airimportgate.ErrInvalidActor {
				// 不需要业务租约即可 ForceAbort 参数校验
				tc.req.LeaseID = "nonexistent-lease"
			} else if berr == nil {
				tc.req.LeaseID = r.Lease.LeaseID
			}
			err := engine.ForceAbort(ctx, tc.req)
			if err != tc.wantErr {
				t.Errorf("want %v, got %v", tc.wantErr, err)
			}
		})
	}
}

// ── 相位冲突证明生成 ──────────────────────────────────────────────────────

func TestEngine_PhaseConflictProof(t *testing.T) {
	baseTime := time.Date(2025, 11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	engine := airimportgate.NewEngine(store, abortedReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	ctx := context.Background()
	gen := airimportgate.MakeImportGen("conflict-uuid-0001", baseTime.Add(-5*time.Second))
	_, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-conflict-001",
		ImportGen:	gen,
		BlobDigest:	airimportgate.DigestBlob([]byte("conflict-blob-001")),
		Actor:		"bob",
	})
	if err != airimportgate.ErrPhaseConflict {
		t.Fatalf("want ErrPhaseConflict, got %v", err)
	}
	// 验证冲突证明已写入
	conflicts, err := engine.ListOpenConflicts(ctx, "sess-conflict-001")
	if err != nil {
		t.Fatalf("ListOpenConflicts: %v", err)
	}
	if len(conflicts) == 0 {
		t.Fatal("expected at least one conflict proof")
	}
	c := conflicts[0]
	if c.ConflictKind != airimportgate.ConflictImportVsAbort {
		t.Errorf("want import_vs_abort, got %s", c.ConflictKind)
	}
	if c.LoserOp != "import" {
		t.Errorf("want loser_op=import, got %s", c.LoserOp)
	}
	if c.LoserActor != "bob" {
		t.Errorf("want loser_actor=bob, got %s", c.LoserActor)
	}
	// 升级冲突证明到 proven
	if err := engine.ProveConflict(ctx, c.ConflictID); err != nil {
		t.Fatalf("ProveConflict: %v", err)
	}
	updated, _ := engine.GetConflict(ctx, c.ConflictID)
	if updated.State != airimportgate.ConflictProven {
		t.Errorf("want proven, got %s", updated.State)
	}
	// 关闭冲突证明
	if err := engine.CloseConflict(ctx, c.ConflictID); err != nil {
		t.Fatalf("CloseConflict: %v", err)
	}
	closed, _ := engine.GetConflict(ctx, c.ConflictID)
	if closed.State != airimportgate.ConflictClosed {
		t.Errorf("want closed, got %s", closed.State)
	}
}

// ── ParseImportGen 单元 ───────────────────────────────────────────────────

func TestParseImportGen(t *testing.T) {
	baseTime := time.Date(2025,
		11, 4, 10, 0, 0, 0, time.UTC)
	cases := []struct {
		name	string
		raw	string
		wantErr	error
	}{
		{
			name:		"有效 gen",
			raw:		airimportgate.MakeImportGen("550e8400-e29b-41d4-a716-446655440000", baseTime),
			wantErr:	nil,
		},
		{
			name:		"空串",
			raw:		"",
			wantErr:	airimportgate.ErrImportGenInvalid,
		},
		{
			name:		"无前缀",
			raw:		"550e8400-e29b-41d4-a716-446655440000-1730678400",
			wantErr:	airimportgate.ErrImportGenInvalid,
		},
		{
			name:		"时间戳非数字",
			raw:		"ig-550e8400-e29b-41d4-a716-446655440000-notanumber",
			wantErr:	airimportgate.ErrImportGenInvalid,
		},
		{
			name:		"时间戳为零（无效）",
			raw:		"ig-550e8400-e29b-41d4-a716-446655440000-0",
			wantErr:	airimportgate.ErrImportGenInvalid,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name,
			func(t *testing.T) {
				_, err := airimportgate.ParseImportGen(tc.raw)
				if err != tc.wantErr {
					t.Errorf("want %v, got %v", tc.wantErr, err)
				}
			})
	}
}

// TestDigestBlob 验证 DigestBlob 为纯函数（同输入同输出）且空 blob 摘要已知。
func TestDigestBlob(t *testing.T) {
	// SHA-256 空串为已知固定值（NIST FIPS 180-4 附录 B）。
	emptyDigest := airimportgate.DigestBlob([]byte{})
	const wantEmpty = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if emptyDigest != wantEmpty {
		t.Errorf("empty blob: want %s, got %s", wantEmpty, emptyDigest)
	}
	// 同输入两次调用必须返回相同摘要（纯函数不变量）。
	d1 := airimportgate.DigestBlob([]byte("sealed-payload-data"))
	d2 := airimportgate.DigestBlob([]byte("sealed-payload-data"))
	if d1 != d2 {
		t.Errorf("DigestBlob not idempotent: %s != %s", d1, d2)
	}
	// 不同输入必须产生不同摘要（抗碰撞基础验证）。
	d3 := airimportgate.DigestBlob([]byte("sealed-payload-data-different"))
	if d1 == d3 {
		t.Error("different inputs produced same digest")
	}
	// 摘要长度必须为 64 个十六进制字符（SHA-256 = 32 字节）。
	if len(d1) != 64 {
		t.Errorf("digest length want 64, got %d", len(d1))
	}
}

// ── Worker 过期吊销 ────────────────────────────────────────────────────────

func TestWorker_ExpiryOnce(t *testing.T) {
	baseTime := time.Date(2025,
		11, 4, 10, 0, 0, 0, time.UTC)
	store := airimportgate.NewMemStore().WithClock(fixedClock(baseTime))
	engine := airimportgate.NewEngine(store, noopReader{}, airimportgate.WithClock(fixedClock(baseTime)))
	ctx := context.Background()
	gen := airimportgate.MakeImportGen("expiry-uuid-0001", baseTime.Add(-5*time.Second))
	r, err := engine.BeginImport(ctx, airimportgate.BeginImportReq{
		SessionID:	"sess-expiry-001",
		ImportGen:	gen,
		BlobDigest:	airimportgate.DigestBlob([]byte("expiry-blob-001")),
		Actor:		"alice",
		LeaseTTL:	5 * time.Second,	// 极短 TTL
	})
	if err != nil {
		t.Fatalf("BeginImport: %v", err)
	}
	// 推进时钟到过期后
	expiredTime := baseTime.Add(10 * time.Second)
	store.WithClock(fixedClock(expiredTime))
	w := airimportgate.NewWorker(engine, store, airimportgate.WorkerConfig{})
	w.clock = fixedClock(expiredTime)
	w.RunExpiryOnce(ctx)
	lease,
		_ := engine.GetLease(ctx, r.Lease.LeaseID)
	if lease.State != airimportgate.LeaseExpired {
		t.Errorf("want expired, got %s", lease.State)
	}
}

// ── MemStore 并发安全基础 ─────────────────────────────────────────────────

func TestMemStore_ConcurrentSafe(t *testing.T) {
	store := airimportgate.NewMemStore()
	ctx := context.Background()
	now := time.Now().UTC()
	done := make(chan struct{}, 10)
	for i := 0; i < 10; i++ {
		i := i
		go func() {
			lease := &airimportgate.ImportLease{
				LeaseID:	"concurrent-lease-" + string(rune('a'+i)),
				SessionID:	"sess-concurrent",
				ImportGen:	airimportgate.MakeImportGen("c-uuid", now),
				State:		airimportgate.LeaseOpen,
				ExpiresAt:	now.Add(time.Minute),
				CreatedAt:	now,
				UpdatedAt:	now,
			}
			_ = store.SaveLease(ctx, lease)
			_, _ = store.GetLease(ctx, lease.LeaseID)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
}
