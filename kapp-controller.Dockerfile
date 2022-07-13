FROM eu.gcr.io/gardener-project/3rd/golang:1.17.9 as builder

ARG kapp_controller_version="v0.14.0"

WORKDIR /go/src/github.com/vmware-tanzu/

RUN apt-get -y install git
RUN git clone --depth 1 --branch ${kapp_controller_version} https://github.com/vmware-tanzu/carvel-kapp-controller.git

WORKDIR /go/src/github.com/vmware-tanzu/carvel-kapp-controller/

RUN apt-get -y update && apt-get -y install ca-certificates && update-ca-certificates

# k14s
RUN wget -O- https://github.com/k14s/ytt/releases/download/v0.41.1/ytt-linux-amd64 > /usr/local/bin/ytt && \
  echo "65dbc4f3a4a2ed84296dd1b323e8e7bd77e488fa7540d12dd36cf7fb2fc77c03  /usr/local/bin/ytt" | sha256sum -c - && \
  chmod +x /usr/local/bin/ytt && ytt version

RUN wget -O- https://github.com/k14s/kapp/releases/download/v0.49.0/kapp-linux-amd64 > /usr/local/bin/kapp && \
  echo "dec5040d90478fdf0af3c1548d46f9ded642f156245bba83fe99171c8461e4f7  /usr/local/bin/kapp" | sha256sum -c - && \
  chmod +x /usr/local/bin/kapp && kapp version

RUN wget -O- https://github.com/k14s/kbld/releases/download/v0.34.0/kbld-linux-amd64 > /usr/local/bin/kbld && \
  echo "67c86ece94a3747b2e011a5b72044b69f2594ca807621b1e1e4c805f6abcaeef  /usr/local/bin/kbld" | sha256sum -c - && \
  chmod +x /usr/local/bin/kbld && kbld version

RUN wget -O- https://github.com/k14s/imgpkg/releases/download/v0.29.0/imgpkg-linux-amd64 > /usr/local/bin/imgpkg && \
  echo "c7190adcb8445480e4e457c899aecdf7ca98606c625493b904c0eb2ab721ce19  /usr/local/bin/imgpkg" | sha256sum -c - && \
  chmod +x /usr/local/bin/imgpkg && imgpkg version

RUN wget -O- https://github.com/vmware-tanzu/carvel-vendir/releases/download/v0.28.1/vendir-linux-amd64 > /usr/local/bin/vendir && \
  echo "9cf05073b88ba702c3ed5be67361fefecef3d34cc16fea684e0b7c09b7b18788  /usr/local/bin/vendir" | shasum -c - && \
  chmod +x /usr/local/bin/vendir && vendir version

# helm
RUN wget -O- https://get.helm.sh/helm-v2.17.0-linux-amd64.tar.gz > /helm && \
  echo "f3bec3c7c55f6a9eb9e6586b8c503f370af92fe987fcbf741f37707606d70296  /helm" | shasum -c - && \
  mkdir /helm-unpacked && tar -C /helm-unpacked -xzvf /helm

# sops
RUN wget -O- https://github.com/mozilla/sops/releases/download/v3.7.3/sops-v3.7.3.linux > /usr/local/bin/sops && \
  echo "53aec65e45f62a769ff24b7e5384f0c82d62668dd96ed56685f649da114b4dbb  /usr/local/bin/sops" | sha256sum -c - && \
  chmod +x /usr/local/bin/sops && sops -v

# kapp-controller
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags=-buildid= -trimpath -o controller ./cmd/main.go

#### BASE ####
FROM gcr.io/distroless/static-debian11:nonroot AS base

# Name it kapp-controller to identify it easier in process tree
COPY --from=builder /go/src/github.com/vmware-tanzu/carvel-kapp-controller/controller kapp-controller

# fetchers
COPY --from=builder /helm-unpacked/linux-amd64/helm .
COPY --from=builder /usr/local/bin/imgpkg .
COPY --from=builder /usr/local/bin/vendir .

# templaters
COPY --from=builder /usr/local/bin/ytt .
COPY --from=builder /usr/local/bin/kbld .
COPY --from=builder /usr/local/bin/sops .

# deployers
COPY --from=builder /usr/local/bin/kapp .

ENV PATH="/:${PATH}"
ENTRYPOINT ["/kapp-controller"]
