// Code generated by gotsrpc https://github.com/foomo/gotsrpc  - DO NOT EDIT.

package walker

import (
	"net/http"

	gotsrpc "github.com/foomo/gotsrpc"
)

type ServiceGoTSRPCClient interface {
	GetResults(filters Filters, page int, pageSize int) (filterOptions FilterOptions, results []ScrapeResult, numPages int, clientErr error)
	GetStatus() (retGetStatus_0 ServiceStatus, clientErr error)
	SetClientEncoding(encoding gotsrpc.ClientEncoding)
	SetTransportHttpClient(client *http.Client)
}

type tsrpcServiceGoTSRPCClient struct {
	URL      string
	EndPoint string
	Client   gotsrpc.Client
}

func NewDefaultServiceGoTSRPCClient(url string) ServiceGoTSRPCClient {
	return NewServiceGoTSRPCClient(url, "/service/walker")
}

func NewServiceGoTSRPCClient(url string, endpoint string) ServiceGoTSRPCClient {
	return NewServiceGoTSRPCClientWithClient(url, "/service/walker", nil)
}

func NewServiceGoTSRPCClientWithClient(url string, endpoint string, client *http.Client) ServiceGoTSRPCClient {
	return &tsrpcServiceGoTSRPCClient{
		URL:      url,
		EndPoint: endpoint,
		Client:   gotsrpc.NewClientWithHttpClient(client),
	}
}

func (tsc *tsrpcServiceGoTSRPCClient) SetClientEncoding(encoding gotsrpc.ClientEncoding) {
	tsc.Client.SetClientEncoding(encoding)
}

func (tsc *tsrpcServiceGoTSRPCClient) SetTransportHttpClient(client *http.Client) {
	tsc.Client.SetTransportHttpClient(client)
}
func (tsc *tsrpcServiceGoTSRPCClient) GetResults(filters Filters, page int, pageSize int) (filterOptions FilterOptions, results []ScrapeResult, numPages int, clientErr error) {
	args := []interface{}{filters, page, pageSize}
	reply := []interface{}{&filterOptions, &results, &numPages}
	clientErr = tsc.Client.Call(tsc.URL, tsc.EndPoint, "GetResults", args, reply)
	return
}

func (tsc *tsrpcServiceGoTSRPCClient) GetStatus() (retGetStatus_0 ServiceStatus, clientErr error) {
	args := []interface{}{}
	reply := []interface{}{&retGetStatus_0}
	clientErr = tsc.Client.Call(tsc.URL, tsc.EndPoint, "GetStatus", args, reply)
	return
}
