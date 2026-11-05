package vo

import "testing"

func TestValidationsAdd(t *testing.T) {
	vs := Validations{}
	vs.add(ValidationLevelInfo, "test", "default")
	if len(vs) != 1 {
		t.Fatal("wrong length")
	}
}
