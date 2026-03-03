package audit

import (
	"context"
	"fmt"
	"time"

	"github.com/bnaylor/perry/internal/storage"
)

// BillingAuditor provides visibility into accumulated costs.
type BillingAuditor struct {
	store *storage.Store
}

// NewBillingAuditor creates a new auditor with the given storage backend.
func NewBillingAuditor(s *storage.Store) *BillingAuditor {
	return &BillingAuditor{store: s}
}

// CheckBudgetStatus returns the current spend for today and the current month.
func (a *BillingAuditor) CheckBudgetStatus(ctx context.Context) (daily, monthly float64, err error) {
	now := time.Now()
	
	// Daily: from midnight
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	daily, err = a.store.TotalSpend(ctx, startOfDay, now)
	if err != nil {
		return 0, 0, fmt.Errorf("daily spend: %w", err)
	}

	// Monthly: from 1st of month
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
	monthly, err = a.store.TotalSpend(ctx, startOfMonth, now)
	if err != nil {
		return 0, 0, fmt.Errorf("monthly spend: %w", err)
	}

	return daily, monthly, nil
}

// BudgetHealth represents the status of current spending vs policy.
type BudgetHealth struct {
	DailySpend     float64
	MonthlySpend   float64
	DailyPercent   float64
	MonthlyPercent float64
	IsHealthy      bool
}

// AssessHealth compares current spend against the policy limits.
func (a *BillingAuditor) AssessHealth(ctx context.Context, maxDaily, maxMonthly float64) (BudgetHealth, error) {
	daily, monthly, err := a.CheckBudgetStatus(ctx)
	if err != nil {
		return BudgetHealth{}, err
	}

	health := BudgetHealth{
		DailySpend:   daily,
		MonthlySpend: monthly,
		IsHealthy:    true,
	}

	if maxDaily > 0 {
		health.DailyPercent = (daily / maxDaily) * 100.0
		if health.DailyPercent > 90.0 {
			health.IsHealthy = false
		}
	}

	if maxMonthly > 0 {
		health.MonthlyPercent = (monthly / maxMonthly) * 100.0
		if health.MonthlyPercent > 90.0 {
			health.IsHealthy = false
		}
	}

	return health, nil
}
