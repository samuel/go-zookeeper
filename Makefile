# make file to hold the logic of build and test setup
ZK_VERSION ?= 3.5.4-beta

ZK = zookeeper-$(ZK_VERSION)
ZK_URL = "https://archive.apache.org/dist/zookeeper/$(ZK)/$(ZK).tar.gz"

$(ZK):
	wget $(ZK_URL)
	tar -zxf $(ZK).tar.gz
	rm $(ZK).tar.gz

.PHONY: install-test-deps
install-test-deps:
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover

.PHONY: travis
travis: $(ZK) install-test-deps