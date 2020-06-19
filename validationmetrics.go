package walker

import (
	"net/url"
	"strings"

	"github.com/foomo/walker/vo"
)

func reportSchemaValidationMetrics(
	completeStatus vo.Status,
	paths []string,
	trackPenalty trackValidationPenalty,
	trackScore trackValidationScore,
) {
	sortedPaths := sortPathsByLength(paths)
ResultLoop:
	for _, r := range completeStatus.Results {
		if r.ValidationReport != nil {
			path := "/"
			for _, p := range sortedPaths {
				u, errParse := url.Parse(r.TargetURL)
				if errParse != nil {
					continue ResultLoop
				}
				if strings.HasPrefix(u.Path, p) {
					path = p
					break
				}
			}
			trackScore(r.Group, path, r.ValidationReport.Score)
			penalties := map[string]int{}
			for _, validation := range r.ValidationReport.Validations {
				penalties[string(validation.Type)] += validation.Penalty
			}

			for validationType, penalty := range penalties {
				trackPenalty(r.Group, path, validationType, penalty)
			}
		}
	}

}
