test-prod-a:
	GOOS=linux go build -o /tmp/test-prod-a cmd/walker/walker.go
	scp -C /tmp/test-prod-a demo.globus-stage.bestbytes.net:~/.
	ssh demo.globus-stage.bestbytes.net ./test-prod-a
test-prod-b:
	GOOS=linux go build -o /tmp/test-prod-b cmd/walker/walker.go
	scp -C /tmp/test-prod-b demo.globus-stage.bestbytes.net:~/.
	ssh demo.globus-stage.bestbytes.net ./test-prod-b
