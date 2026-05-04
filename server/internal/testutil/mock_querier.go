// Package testutil provides shared test helpers, including mock implementations
// of interfaces used across the server packages.
package testutil

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/Octopoos42/shellai/server/internal/db"
)

// MockQuerier is a test double for db.Querier. Each method delegates to an
// optional function field; unset fields return zero values and nil errors.
type MockQuerier struct {
	GetAPIKeyByHashFn func(ctx context.Context, keyHash string) (db.ApiKey, error)
	CreateAPIKeyFn    func(ctx context.Context, arg db.CreateAPIKeyParams) (db.ApiKey, error)
	ListAPIKeysFn     func(ctx context.Context) ([]db.ApiKey, error)
	RevokeAPIKeyFn    func(ctx context.Context, id pgtype.UUID) (db.ApiKey, error)

	CreateSessionFn      func(ctx context.Context, arg db.CreateSessionParams) (db.Session, error)
	GetSessionFn         func(ctx context.Context, id pgtype.UUID) (db.Session, error)
	ListSessionsFn       func(ctx context.Context, apiKeyID pgtype.UUID) ([]db.Session, error)
	UpdateSessionTitleFn func(ctx context.Context, arg db.UpdateSessionTitleParams) (db.Session, error)
	DeleteSessionFn      func(ctx context.Context, id pgtype.UUID) error
	TouchSessionFn       func(ctx context.Context, id pgtype.UUID) error

	CreateMessageFn           func(ctx context.Context, arg db.CreateMessageParams) (db.Message, error)
	ListMessagesBySessionFn   func(ctx context.Context, sessionID pgtype.UUID) ([]db.Message, error)
	DeleteMessagesBySessionFn func(ctx context.Context, sessionID pgtype.UUID) error

	CreateSkillFn       func(ctx context.Context, arg db.CreateSkillParams) (db.Skill, error)
	GetSkillFn          func(ctx context.Context, id pgtype.UUID) (db.Skill, error)
	ListSkillsByOwnerFn func(ctx context.Context, apiKeyID pgtype.UUID) ([]db.Skill, error)
	ListPublicSkillsFn  func(ctx context.Context) ([]db.Skill, error)
	UpdateSkillFn       func(ctx context.Context, arg db.UpdateSkillParams) (db.Skill, error)
	DeleteSkillFn       func(ctx context.Context, id pgtype.UUID) error
}

func (m *MockQuerier) GetAPIKeyByHash(ctx context.Context, keyHash string) (db.ApiKey, error) {
	if m.GetAPIKeyByHashFn != nil {
		return m.GetAPIKeyByHashFn(ctx, keyHash)
	}
	return db.ApiKey{}, nil
}

func (m *MockQuerier) CreateAPIKey(ctx context.Context, arg db.CreateAPIKeyParams) (db.ApiKey, error) {
	if m.CreateAPIKeyFn != nil {
		return m.CreateAPIKeyFn(ctx, arg)
	}
	return db.ApiKey{}, nil
}

func (m *MockQuerier) ListAPIKeys(ctx context.Context) ([]db.ApiKey, error) {
	if m.ListAPIKeysFn != nil {
		return m.ListAPIKeysFn(ctx)
	}
	return nil, nil
}

func (m *MockQuerier) RevokeAPIKey(ctx context.Context, id pgtype.UUID) (db.ApiKey, error) {
	if m.RevokeAPIKeyFn != nil {
		return m.RevokeAPIKeyFn(ctx, id)
	}
	return db.ApiKey{}, nil
}

func (m *MockQuerier) CreateSession(ctx context.Context, arg db.CreateSessionParams) (db.Session, error) {
	if m.CreateSessionFn != nil {
		return m.CreateSessionFn(ctx, arg)
	}
	return db.Session{}, nil
}

func (m *MockQuerier) GetSession(ctx context.Context, id pgtype.UUID) (db.Session, error) {
	if m.GetSessionFn != nil {
		return m.GetSessionFn(ctx, id)
	}
	return db.Session{}, nil
}

func (m *MockQuerier) ListSessions(ctx context.Context, apiKeyID pgtype.UUID) ([]db.Session, error) {
	if m.ListSessionsFn != nil {
		return m.ListSessionsFn(ctx, apiKeyID)
	}
	return nil, nil
}

func (m *MockQuerier) UpdateSessionTitle(ctx context.Context, arg db.UpdateSessionTitleParams) (db.Session, error) {
	if m.UpdateSessionTitleFn != nil {
		return m.UpdateSessionTitleFn(ctx, arg)
	}
	return db.Session{}, nil
}

func (m *MockQuerier) DeleteSession(ctx context.Context, id pgtype.UUID) error {
	if m.DeleteSessionFn != nil {
		return m.DeleteSessionFn(ctx, id)
	}
	return nil
}

func (m *MockQuerier) TouchSession(ctx context.Context, id pgtype.UUID) error {
	if m.TouchSessionFn != nil {
		return m.TouchSessionFn(ctx, id)
	}
	return nil
}

func (m *MockQuerier) CreateMessage(ctx context.Context, arg db.CreateMessageParams) (db.Message, error) {
	if m.CreateMessageFn != nil {
		return m.CreateMessageFn(ctx, arg)
	}
	return db.Message{}, nil
}

func (m *MockQuerier) ListMessagesBySession(ctx context.Context, sessionID pgtype.UUID) ([]db.Message, error) {
	if m.ListMessagesBySessionFn != nil {
		return m.ListMessagesBySessionFn(ctx, sessionID)
	}
	return nil, nil
}

func (m *MockQuerier) DeleteMessagesBySession(ctx context.Context, sessionID pgtype.UUID) error {
	if m.DeleteMessagesBySessionFn != nil {
		return m.DeleteMessagesBySessionFn(ctx, sessionID)
	}
	return nil
}

func (m *MockQuerier) CreateSkill(ctx context.Context, arg db.CreateSkillParams) (db.Skill, error) {
	if m.CreateSkillFn != nil {
		return m.CreateSkillFn(ctx, arg)
	}
	return db.Skill{}, nil
}

func (m *MockQuerier) GetSkill(ctx context.Context, id pgtype.UUID) (db.Skill, error) {
	if m.GetSkillFn != nil {
		return m.GetSkillFn(ctx, id)
	}
	return db.Skill{}, nil
}

func (m *MockQuerier) ListSkillsByOwner(ctx context.Context, apiKeyID pgtype.UUID) ([]db.Skill, error) {
	if m.ListSkillsByOwnerFn != nil {
		return m.ListSkillsByOwnerFn(ctx, apiKeyID)
	}
	return nil, nil
}

func (m *MockQuerier) ListPublicSkills(ctx context.Context) ([]db.Skill, error) {
	if m.ListPublicSkillsFn != nil {
		return m.ListPublicSkillsFn(ctx)
	}
	return nil, nil
}

func (m *MockQuerier) UpdateSkill(ctx context.Context, arg db.UpdateSkillParams) (db.Skill, error) {
	if m.UpdateSkillFn != nil {
		return m.UpdateSkillFn(ctx, arg)
	}
	return db.Skill{}, nil
}

func (m *MockQuerier) DeleteSkill(ctx context.Context, id pgtype.UUID) error {
	if m.DeleteSkillFn != nil {
		return m.DeleteSkillFn(ctx, id)
	}
	return nil
}
