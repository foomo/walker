package walker

type ServiceStatus struct {
	TargetURL string
	Open      int
	Done      int
	Pending   int
}
