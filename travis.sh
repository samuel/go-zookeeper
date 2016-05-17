#!/bin/bash

set -x

: ${ZK_VERSION="3.4.6"}

case $1 in
	-install)
		echo "install dependencies for travis"
		go get github.com/golang/lint/golint
		go get github.com/GeertJohan/fgt
		go get golang.org/x/tools/cmd/cover
		go get github.com/mattn/goveralls
		wget "http://apache.cs.utah.edu/zookeeper/zookeeper-${ZK_VERSION}/zookeeper-${ZK_VERSION}.tar.gz"
		tar -xvf "zookeeper-${ZK_VERSION}.tar.gz"
		#mv zookeeper-$ZK_VERSION zk
		#mv ./zk/conf/zoo_sample.cfg ./zk/conf/zoo.cfg
		# ./zk/bin/zkServer.sh start ./zk/conf/zoo.cfg 1> /dev/null
		;;
esac
