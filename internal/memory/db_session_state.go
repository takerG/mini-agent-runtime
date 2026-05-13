package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// DBSessionStateMemory 模拟 DB session state 存储，首版仅使用本地内存 map。
type DBSessionStateMemory struct {
	state map[string]map[string]string
}

// NewDBSessionStateMemory 创建本地模拟的 DB session state memory。
func NewDBSessionStateMemory() *DBSessionStateMemory {
	return &DBSessionStateMemory{state: make(map[string]map[string]string)}
}

// Name 返回 DB session state memory 的 provider 名称。
func (m *DBSessionStateMemory) Name() string {
	return "db_session_state"
}

// Scope 返回 DB session state memory 的归属范围。
func (m *DBSessionStateMemory) Scope() Scope {
	return ScopeSession
}

// AppendTurn 保持 Provider 接口一致，DB session state 不自动记录完整对话。
func (m *DBSessionStateMemory) AppendTurn(context.Context, Query, Turn) error {
	return nil
}

// Set 写入指定 session 的状态键值。
func (m *DBSessionStateMemory) Set(_ context.Context, sessionID string, key string, value string) error {
	if m == nil || sessionID == "" || key == "" {
		return nil
	}
	if _, ok := m.state[sessionID]; !ok {
		m.state[sessionID] = make(map[string]string)
	}
	m.state[sessionID][key] = value
	return nil
}

// Get 读取指定 session 的状态键值。
func (m *DBSessionStateMemory) Get(_ context.Context, sessionID string, key string) (string, bool, error) {
	if m == nil || sessionID == "" || key == "" {
		return "", false, nil
	}
	values, ok := m.state[sessionID]
	if !ok {
		return "", false, nil
	}
	value, ok := values[key]
	return value, ok, nil
}

// Snapshot 返回指定 session 的状态副本。
func (m *DBSessionStateMemory) Snapshot(_ context.Context, sessionID string) (map[string]string, error) {
	if m == nil || sessionID == "" {
		return map[string]string{}, nil
	}
	values := m.state[sessionID]
	snapshot := make(map[string]string, len(values))
	for key, value := range values {
		snapshot[key] = value
	}
	return snapshot, nil
}

// ContextBlock 返回指定 session state 的可读上下文。
func (m *DBSessionStateMemory) ContextBlock(ctx context.Context, query Query) (Block, bool, error) {
	snapshot, err := m.Snapshot(ctx, query.SessionID)
	if err != nil {
		return Block{}, false, err
	}
	if len(snapshot) == 0 {
		return Block{}, false, nil
	}
	keys := make([]string, 0, len(snapshot))
	for key := range snapshot {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var builder strings.Builder
	for _, key := range keys {
		fmt.Fprintf(&builder, "%s=%s\n", key, snapshot[key])
	}
	return Block{
		Provider: m.Name(),
		Scope:    ScopeSession,
		Content:  strings.TrimSpace(builder.String()),
	}, true, nil
}
