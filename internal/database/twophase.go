package database

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"gorm.io/gorm"
)

type Participant struct {
	Name   string
	DB     *gorm.DB
	Tx     *gorm.DB
	Status string
}

type TwoPhaseManager struct {
	mu           sync.Mutex
	participants map[string]*Participant
	timeout      time.Duration
	logger       func(string, ...interface{})
}

func NewTwoPhaseManager(timeout time.Duration) *TwoPhaseManager {
	return &TwoPhaseManager{
		participants: make(map[string]*Participant),
		timeout:      timeout,
		logger: func(format string, args ...interface{}) {
			fmt.Printf("[2PC] "+format+"\n", args...)
		},
	}
}

func (m *TwoPhaseManager) Register(name string, db *gorm.DB) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.participants[name]; exists {
		return fmt.Errorf("participant %s already registered", name)
	}
	m.participants[name] = &Participant{Name: name, DB: db, Status: "idle"}
	return nil
}

func (m *TwoPhaseManager) Begin(ctx context.Context) (map[string]*gorm.DB, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	txs := make(map[string]*gorm.DB, len(m.participants))
	started := make([]string, 0)

	for name, p := range m.participants {
		tx := p.DB.WithContext(ctx).Begin()
		if tx.Error != nil {
			for _, n := range started {
				_ = m.participants[n].Tx.Rollback()
				m.participants[n].Status = "idle"
			}
			return nil, fmt.Errorf("begin tx for %s failed: %w", name, tx.Error)
		}
		p.Tx = tx
		p.Status = "active"
		txs[name] = tx
		started = append(started, name)
	}
	m.logger("begin %d transactions", len(started))
	return txs, nil
}

func (m *TwoPhaseManager) Prepare() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for name, p := range m.participants {
		if p.Status != "active" {
			continue
		}
		dialect := p.DB.Dialector.Name()
		switch dialect {
		case "postgres":
			if err := p.Tx.Exec("PREPARE TRANSACTION ?", fmt.Sprintf("xa_%d_%s", time.Now().UnixNano(), name)).Error; err != nil {
				m.logger("prepare %s failed: %v, triggering rollback", name, err)
				_ = m.rollbackLocked()
				return fmt.Errorf("prepare %s: %w", name, err)
			}
		case "mysql":
			if err := p.Tx.Exec("XA PREPARE ?", fmt.Sprintf("xa_%d_%s", time.Now().UnixNano(), name)).Error; err != nil {
				m.logger("prepare %s failed: %v, triggering rollback", name, err)
				_ = m.rollbackLocked()
				return fmt.Errorf("prepare %s: %w", name, err)
			}
		default:
			m.logger("dialect %s does not support XA, skip PREPARE for %s", dialect, name)
		}
		p.Status = "prepared"
	}
	m.logger("all participants prepared")
	return nil
}

func (m *TwoPhaseManager) Commit() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var firstErr error
	for name, p := range m.participants {
		if p.Status != "prepared" && p.Status != "active" {
			continue
		}
		if err := p.Tx.Commit().Error; err != nil {
			m.logger("commit %s failed: %v", name, err)
			if firstErr == nil {
				firstErr = fmt.Errorf("commit %s: %w", name, err)
			}
			continue
		}
		p.Status = "committed"
	}
	if firstErr != nil {
		m.logger("commit completed with errors: %v", firstErr)
		return firstErr
	}
	m.logger("all participants committed")
	return nil
}

func (m *TwoPhaseManager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.rollbackLocked()
}

func (m *TwoPhaseManager) rollbackLocked() error {
	var firstErr error
	for name, p := range m.participants {
		if p.Status == "idle" || p.Status == "committed" || p.Status == "rolledback" {
			continue
		}
		if p.Tx != nil {
			if err := p.Tx.Rollback().Error; err != nil && !errors.Is(err, gorm.ErrInvalidTransaction) {
				m.logger("rollback %s failed: %v", name, err)
				if firstErr == nil {
					firstErr = fmt.Errorf("rollback %s: %w", name, err)
				}
				continue
			}
		}
		p.Status = "rolledback"
	}
	m.logger("rollback completed")
	return firstErr
}

func (m *TwoPhaseManager) Execute(ctx context.Context, fn func(txs map[string]*gorm.DB) error) error {
	txs, err := m.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			_ = m.Rollback()
			panic(r)
		}
	}()

	if err := fn(txs); err != nil {
		_ = m.Rollback()
		return err
	}

	if err := m.Prepare(); err != nil {
		return err
	}
	return m.Commit()
}

func (m *TwoPhaseManager) Statuses() map[string]string {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := make(map[string]string, len(m.participants))
	for k, v := range m.participants {
		s[k] = v.Status
	}
	return s
}
