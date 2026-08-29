package auth

import (
	"context"
	"time"
)

type User struct{ ID, GoogleSub, Email, Name, Picture string }

type Session struct {
	AccessToken  string
	RefreshToken string
	ExpiresAt    time.Time
	User         User
}

type UserStore interface {
	UpsertUser(ctx context.Context, u User) error
	UserByGoogleSub(ctx context.Context, sub string) (User, bool, error)
	UserByID(ctx context.Context, id string) (User, bool, error)
	SaveRefresh(ctx context.Context, hash, userID string, expiresAt time.Time) error
	RefreshOwner(ctx context.Context, hash string) (string, bool, error)
	DeleteRefresh(ctx context.Context, hash string) error
}

type Service struct {
	v     Verifier
	t     *Tokens
	store UserStore
	newID func() string
}

func NewService(v Verifier, t *Tokens, s UserStore, newID func() string) *Service {
	return &Service{v: v, t: t, store: s, newID: newID}
}

// LoginAs находит-или-создаёт пользователя по Identity и выдаёт сессию
// (access+refresh). Первый вход = регистрация.
func (s *Service) LoginAs(ctx context.Context, id Identity) (Session, error) {
	u, ok, err := s.store.UserByGoogleSub(ctx, id.Sub)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		u = User{ID: s.newID(), GoogleSub: id.Sub, Email: id.Email, Name: id.Name, Picture: id.Picture}
		if err := s.store.UpsertUser(ctx, u); err != nil {
			return Session{}, err
		}
	}
	return s.issue(ctx, u)
}

// GoogleLogin проверяет ID-token, находит-или-создаёт пользователя и выдаёт
// сессию (access+refresh). Первый вход = регистрация.
func (s *Service) GoogleLogin(ctx context.Context, idToken string) (Session, error) {
	id, err := s.v.Verify(ctx, idToken)
	if err != nil {
		return Session{}, err
	}
	return s.LoginAs(ctx, id)
}

// Refresh обменивает refresh на новую пару, гася старый (ротация).
func (s *Service) Refresh(ctx context.Context, refresh string) (Session, error) {
	hash := HashRefresh(refresh)
	uid, ok, err := s.store.RefreshOwner(ctx, hash)
	if err != nil {
		return Session{}, err
	}
	if !ok {
		return Session{}, ErrInvalid
	}
	if err := s.store.DeleteRefresh(ctx, hash); err != nil {
		return Session{}, err
	}
	u, ok, err := s.store.UserByID(ctx, uid)
	if err != nil || !ok {
		return Session{}, ErrInvalid
	}
	return s.issue(ctx, u)
}

func (s *Service) Me(ctx context.Context, userID string) (User, bool, error) {
	return s.store.UserByID(ctx, userID)
}

// Verify проверяет access-токен и возвращает userID (Subject). Используется
// мидлварью requireAuth на HTTP-грани.
func (s *Service) Verify(access string) (string, error) {
	return s.t.VerifyAccess(access)
}

func (s *Service) issue(ctx context.Context, u User) (Session, error) {
	access, exp, err := s.t.IssueAccess(u.ID)
	if err != nil {
		return Session{}, err
	}
	raw, hash, rexp, err := s.t.NewRefresh()
	if err != nil {
		return Session{}, err
	}
	if err := s.store.SaveRefresh(ctx, hash, u.ID, rexp); err != nil {
		return Session{}, err
	}
	return Session{AccessToken: access, RefreshToken: raw, ExpiresAt: exp, User: u}, nil
}
