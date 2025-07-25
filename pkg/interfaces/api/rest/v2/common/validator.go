package common

import (
	"github.com/go-playground/validator/v10"
)

// Validator wraps the go-playground validator
type Validator struct {
	validator *validator.Validate
}

// NewValidator creates a new validator instance
func NewValidator() *Validator {
	v := validator.New()

	// Register custom validators
	v.RegisterValidation("status", validateStatus)
	v.RegisterValidation("campaign_status", validateCampaignStatus)
	v.RegisterValidation("job_status", validateJobStatus)
	v.RegisterValidation("bot_status", validateBotStatus)

	return &Validator{
		validator: v,
	}
}

// ValidateStruct validates a struct
func (v *Validator) ValidateStruct(s interface{}) error {
	return v.validator.Struct(s)
}

// Custom validation functions

func validateStatus(fl validator.FieldLevel) bool {
	status := fl.Field().String()
	validStatuses := []string{"active", "inactive", "completed", "failed", "pending"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

func validateCampaignStatus(fl validator.FieldLevel) bool {
	status := fl.Field().String()
	validStatuses := []string{"draft", "active", "paused", "completed", "failed"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

func validateJobStatus(fl validator.FieldLevel) bool {
	status := fl.Field().String()
	validStatuses := []string{"pending", "assigned", "running", "completed", "failed", "cancelled"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}

func validateBotStatus(fl validator.FieldLevel) bool {
	status := fl.Field().String()
	validStatuses := []string{"idle", "busy", "offline", "error"}
	for _, valid := range validStatuses {
		if status == valid {
			return true
		}
	}
	return false
}
