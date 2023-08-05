# Source(with modifications): https://github.com/ceph/go-ceph/blob/master/testing/containers/ceph/Dockerfile
ARG CEPH_IMG=quay.io/ceph/ceph
ARG CEPH_TAG=v14.2.6
FROM ${CEPH_IMG}:${CEPH_TAG}

RUN true && \
    yum update -y && \
    cv="$(rpm -q --queryformat '%{version}-%{release}' ceph-common)" && \
    yum install -y \
        git wget curl make \
        /usr/bin/cc /usr/bin/c++ \
        "libcephfs-devel-${cv}" "librados-devel-${cv}" "librbd-devel-${cv}" && \
    yum clean all && \
    true

ENV GO_VERSION=1.20.6

COPY go1.20.6.linux-amd64.tar.gz /tmp/

RUN true && \
    gotar=go1.20.6.linux-amd64.tar.gz && \
    tar -x -C /opt/ -f /tmp/${gotar} && \
    rm -f /tmp/${gotar} && \
    true

ENV PATH="${PATH}:/opt/go/bin"
ENV GOROOT=/opt/go
ENV GO111MODULE=on
ENV GOPATH /go
ENV GODEBUG=cgocheck=2
ENV GOCACHE=/go/cache
WORKDIR /opt/s3-operator
VOLUME /opt/s3-operator

COPY micro-osd.sh /
COPY entrypoint.sh /
ENTRYPOINT ["/entrypoint.sh"]
