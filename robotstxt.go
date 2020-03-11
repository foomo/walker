package walker

import (
	"net/http"

	"github.com/temoto/robotstxt"
)

func getRobotsData(baseURL string) (data *robotstxt.RobotsData, err error) {
	resp, errGet := http.Get(baseURL + "/robots.txt")
	if errGet != nil {
		return nil, errGet
	}
	data, errFromResponse := robotstxt.FromResponse(resp)
	if errFromResponse != nil {
		return nil, errFromResponse
	}
	return data, nil
}
