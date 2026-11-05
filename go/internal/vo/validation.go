package vo

type ValidationLevel string

const (
	ValidationLevelError   ValidationLevel = "error"
	ValidationLevelWarning ValidationLevel = "warning"
	ValidationLevelInfo    ValidationLevel = "info"
)

// ResultValidation is a finding of a ValidationFunc for a scrape result
type ResultValidation struct {
	Level   ValidationLevel
	Message string
	Group   string
}

type Validations []ResultValidation

func (v *Validations) add(level ValidationLevel, msg string, group string) {
	*v = append(*v, ResultValidation{Level: level, Group: group, Message: msg})
}

func (v *Validations) Error(group, msg string) {
	v.add(ValidationLevelError, msg, group)
}

func (v *Validations) Warning(group, msg string) {
	v.add(ValidationLevelWarning, msg, group)
}

func (v *Validations) Info(group, msg string) {
	v.add(ValidationLevelInfo, msg, group)
}

func (v *Validations) Group(group string) (err func(msg string), warning func(msg string), info func(msg string)) {
	err = func(msg string) { v.Error(group, msg) }
	warning = func(msg string) { v.Warning(group, msg) }
	info = func(msg string) { v.Info(group, msg) }
	return
}
