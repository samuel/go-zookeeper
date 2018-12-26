# make file to hold the logic of build and test setup
ZK_VERSION ?= 3.4.12

ZK = zookeeper-$(ZK_VERSION)
ZK_URL = "https://archive.apache.org/dist/zookeeper/$(ZK)/$(ZK).tar.gz"

$(ZK):
	wget $(ZK_URL)
	tar -zxf $(ZK).tar.gz
	# we link to a standard directory path so then the tests dont need to find based on version
	ln -s $(ZK) zookeeper
	# in older versions we use the zk fatjar and need to remove the signature from the jar to run it.
	-zip -d zookeeper/contrib/fatjar/zookeeper-${zk_version}-fatjar.jar 'META-INF/*.SF' 'META-INF/*.DSA'


.PHONY: install-test-deps
install-test-deps:
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover

.PHONY: travis
travis: $(ZK) install-test-deps

.PHONY: test
test:
	go test -timeout 120s -v ./...
	go test -timeout 300s -v -i -race ./...
	go test -timeout 300s -v -race -covermode atomic -coverprofile=profile.cov ./zk
	# ignore if we fail to publish coverage
	-goveralls -coverprofile=profile.cov -service=travis-ci
