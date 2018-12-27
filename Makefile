# make file to hold the logic of build and test setup
ZK_VERSION ?= 3.4.12

ZK = zookeeper-$(ZK_VERSION)
ZK_URL = "https://archive.apache.org/dist/zookeeper/$(ZK)/$(ZK).tar.gz"

$(ZK):
	wget $(ZK_URL)
	tar -zxf $(ZK).tar.gz
	# we link to a standard directory path so then the tests dont need to find based on version
	# in the test code. this allows backward compatable testing.
	ln -s $(ZK) zookeeper


.PHONY: install-test-deps
install-test-deps:
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover

.PHONY: travis
travis: $(ZK) install-test-deps

.PHONY: test
test:
	go test -timeout 300s -v ./...
	go test -timeout 400s -v -i -race ./...
	go test -timeout 400s -v -race -covermode atomic -coverprofile=profile.cov ./zk
	# ignore if we fail to publish coverage
	-goveralls -coverprofile=profile.cov -service=travis-ci
