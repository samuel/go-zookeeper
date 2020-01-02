# make file to hold the logic of build and test setup
ZK_VERSION ?= 3.5.6

ZK = zookeeper-$(ZK_VERSION)
ZK_PREFIX ?= apache-
ZK_URL = "https://archive.apache.org/dist/zookeeper/$(ZK)/$(ZK_PREFIX)$(ZK).tar.gz"

tls_passwd = password
tls_dir = "/tmp/certs"

PACKAGES := $(shell go list ./... | grep -v examples)

.DEFAULT_GOAL := test

$(ZK_PREFIX)$(ZK):
	wget $(ZK_URL)
	tar -zxf $(ZK_PREFIX)$(ZK).tar.gz
	# we link to a standard directory path so then the tests dont need to find based on version
	# in the test code. this allows backward compatable testing.
	ln -s $(ZK_PREFIX)$(ZK) zookeeper

.PHONY: install-covertools
install-covertools:
	go get github.com/mattn/goveralls
	go get golang.org/x/tools/cmd/cover

.PHONY: setup
setup: certs $(ZK_PREFIX)$(ZK) install-covertools


.PHONY: lint
lint:
	go fmt ./...
	go vet ./...

.PHONY: build
build:
	go build ./...

.PHONY: test
test: build
	go test -timeout 500s -v -race -covermode atomic -coverprofile=profile.cov $(PACKAGES)
	# ignore if we fail to publish coverage
	-goveralls -coverprofile=profile.cov -service=travis-ci

.PHONY: certs
certs:
	mkdir $(tls_dir)
	keytool -keystore $(tls_dir)/zookeeper.server.keystore.jks -storepass $(tls_passwd)  -alias localhost -validity 10 -genkey -keyalg RSA -dname "CN=localhost, OU=nope, O=nope, L=nope, S=nope, C=NO"
	openssl req -passout pass:$(tls_passwd) -new -x509 -keyout $(tls_dir)/ca-key -out $(tls_dir)/ca-cert -days 10 -subj "/CN=localhost/OU=nope/O=nope/L=nope/C=NO"
	keytool -keystore $(tls_dir)/zookeeper.client.truststore.jks -alias CARoot -import -file $(tls_dir)/ca-cert -trustcacerts -noprompt -storepass $(tls_passwd)
	keytool -keystore $(tls_dir)/zookeeper.server.keystore.jks -alias localhost -certreq -file $(tls_dir)/cert-file -storepass $(tls_passwd)
	openssl x509 -req -CA $(tls_dir)/ca-cert -CAkey $(tls_dir)/ca-key -in $(tls_dir)/cert-file -out $(tls_dir)/cert-signed -days 10 -CAcreateserial -passin pass:$(tls_passwd)
	keytool -keystore $(tls_dir)/zookeeper.server.keystore.jks -alias CARoot -import -file $(tls_dir)/ca-cert -storepass $(tls_passwd) -trustcacerts -noprompt
	keytool -keystore $(tls_dir)/zookeeper.server.keystore.jks -alias localhost -import -file $(tls_dir)/cert-signed -storepass $(tls_passwd)
	keytool -keystore $(tls_dir)/zookeeper.client.keystore.jks -alias CLIENT -validity 10 -genkey -keyalg rsa -storepass $(tls_passwd) -dname "CN=localhost, OU=nope, O=nope, L=nope, S=nope, C=NO"
	keytool -keystore $(tls_dir)/zookeeper.client.keystore.jks -alias CLIENT -certreq -file $(tls_dir)/client-cert-file -storepass $(tls_passwd)
	openssl x509 -req -CA $(tls_dir)/ca-cert -CAkey $(tls_dir)/ca-key -in $(tls_dir)/client-cert-file -out $(tls_dir)/client-cert-signed -days 10 -CAcreateserial -passin pass:$(tls_passwd)
	keytool -keystore $(tls_dir)/zookeeper.client.keystore.jks -alias CARoot -import -file $(tls_dir)/ca-cert -storepass $(tls_passwd) -trustcacerts -noprompt
	keytool -keystore $(tls_dir)/zookeeper.client.keystore.jks -alias CLIENT -import -file $(tls_dir)/client-cert-signed -storepass $(tls_passwd) -trustcacerts -noprompt
	keytool -keystore $(tls_dir)/zookeeper.server.truststore.jks -alias CARoot -import -file $(tls_dir)/ca-cert -storepass $(tls_passwd) -trustcacerts -noprompt
	keytool -storepass $(tls_passwd) -srcstorepass $(tls_passwd) -importkeystore -srckeystore $(tls_dir)/zookeeper.server.truststore.jks -destkeystore $(tls_dir)/server.p12 -deststoretype PKCS12 -noprompt
	openssl pkcs12 -in $(tls_dir)/server.p12 -nokeys -out $(tls_dir)/server.cer.pem -passin pass:$(tls_passwd)
	keytool -importkeystore -srckeystore $(tls_dir)/zookeeper.server.keystore.jks -destkeystore $(tls_dir)/client.p12 -deststoretype PKCS12 -storepass $(tls_passwd) -srcstorepass $(tls_passwd)
	openssl pkcs12 -in $(tls_dir)/client.p12 -nokeys -out $(tls_dir)/client.cer.pem -passin pass:$(tls_passwd)
	openssl pkcs12 -in $(tls_dir)/client.p12 -nodes -nocerts -out $(tls_dir)/client.key.pem -passin pass:$(tls_passwd)