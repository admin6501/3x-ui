package service

import (
	"strings"

	"github.com/mhsanaei/3x-ui/v3/internal/database"
	"github.com/mhsanaei/3x-ui/v3/internal/database/model"
	"github.com/mhsanaei/3x-ui/v3/internal/util/common"
)

// PlanService manages reusable client package/plan templates.
type PlanService struct{}

// List returns every plan ordered by sort order then name.
func (s *PlanService) List() ([]model.Plan, error) {
	var plans []model.Plan
	err := database.GetDB().Order("sort_order asc, name asc").Find(&plans).Error
	return plans, err
}

// Get returns a single plan by id.
func (s *PlanService) Get(id int) (*model.Plan, error) {
	var plan model.Plan
	if err := database.GetDB().First(&plan, id).Error; err != nil {
		return nil, err
	}
	return &plan, nil
}

func validatePlan(p *model.Plan) error {
	p.Name = strings.TrimSpace(p.Name)
	if p.Name == "" {
		return common.NewError("plan name is required")
	}
	if p.DurationDays < 0 || p.TotalGB < 0 || p.LimitIp < 0 || p.Reset < 0 {
		return common.NewError("plan values cannot be negative")
	}
	return nil
}

// Create inserts a new plan.
func (s *PlanService) Create(p *model.Plan) error {
	if err := validatePlan(p); err != nil {
		return err
	}
	p.Id = 0
	return database.GetDB().Create(p).Error
}

// Update overwrites an existing plan's editable fields.
func (s *PlanService) Update(id int, in *model.Plan) error {
	if err := validatePlan(in); err != nil {
		return err
	}
	return database.GetDB().Model(&model.Plan{}).Where("id = ?", id).Updates(map[string]any{
		"name":          in.Name,
		"duration_days": in.DurationDays,
		"total_gb":      in.TotalGB,
		"limit_ip":      in.LimitIp,
		"reset":         in.Reset,
		"enable":        in.Enable,
		"remark":        in.Remark,
		"sort_order":    in.SortOrder,
	}).Error
}

// Delete removes a plan by id.
func (s *PlanService) Delete(id int) error {
	return database.GetDB().Delete(&model.Plan{}, id).Error
}
