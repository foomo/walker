package walker

const (
	ErrorHasBrokenLinks = "ErrorHasBrokenLinks"
)

type ServiceStatus struct {
	TargetURL string
	Open      int
	Done      int
	Pending   int
}
