FROM eu.gcr.io/gardener-project/3rd/golang:1.15.5

ARG kapp_controller_version="v0.14.0"

WORKDIR /go/src/github.com/vmware-tanzu/

RUN apt-get -y install git
RUN git clone --depth 1 --branch ${kapp_controller_version} https://github.com/vmware-tanzu/carvel-kapp-controller.git

WORKDIR /go/src/github.com/vmware-tanzu/carvel-kapp-controller/

RUN apt-get -y update && apt-get -y install ca-certificates && update-ca-certificates

# k14s
RUN wget -O- https://github.com/k14s/ytt/releases/download/v0.30.0/ytt-linux-amd64 > /usr/local/bin/ytt && \
  echo "456e58c70aef5cd4946d29ed106c2b2acbb4d0d5e99129e526ecb4a859a36145  /usr/local/bin/ytt" | sha256sum -c - && \
  chmod +x /usr/local/bin/ytt && ytt version

RUN wget -O- https://github.com/k14s/kapp/releases/download/v0.34.0/kapp-linux-amd64 > /usr/local/bin/kapp && \
  echo "e170193c40ff5dff9f9274c25048de1f50e23c69e8406df274fbb416d5862d7f  /usr/local/bin/kapp" | sha256sum -c - && \
  chmod +x /usr/local/bin/kapp && kapp version

RUN wget -O- https://github.com/k14s/kbld/releases/download/v0.24.0/kbld-linux-amd64 > /usr/local/bin/kbld && \
  echo "63f06c428cacd66e4ebbd23df3f04214109bc44ee623c7c81ecb9aa35c192c65  /usr/local/bin/kbld" | sha256sum -c - && \
  chmod +x /usr/local/bin/kbld && kbld version

RUN wget -O- https://github.com/k14s/imgpkg/releases/download/v0.2.0/imgpkg-linux-amd64 > /usr/local/bin/imgpkg && \
  echo "57a73c4721c39f815408f486c1acfb720af82450996e2bfdf4c2c280d8a28dcc  /usr/local/bin/imgpkg" | sha256sum -c - && \
  chmod +x /usr/local/bin/imgpkg && imgpkg version

RUN wget -O- https://github.com/vmware-tanzu/carvel-vendir/releases/download/v0.14.0/vendir-linux-amd64 > /usr/local/bin/vendir && \
  echo "c224bdfe74df326d7e75b4c50669ec5976b95c0ff9a27d25c6e1833d0c781946  /usr/local/bin/vendir" | shasum -c - && \
  chmod +x /usr/local/bin/vendir && vendir version

# helm
RUN wget -O- https://get.helm.sh/helm-v2.17.0-linux-amd64.tar.gz > /helm && \
  echo "f3bec3c7c55f6a9eb9e6586b8c503f370af92fe987fcbf741f37707606d70296  /helm" | shasum -c - && \
  mkdir /helm-unpacked && tar -C /helm-unpacked -xzvf /helm

# sops
RUN wget -O- https://github.com/mozilla/sops/releases/download/v3.6.1/sops-v3.6.1.linux > /usr/local/bin/sops && \
  echo "b2252aa00836c72534471e1099fa22fab2133329b62d7826b5ac49511fcc8997  /usr/local/bin/sops" | sha256sum -c - && \
  chmod +x /usr/local/bin/sops && sops -v

# kapp-controller
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags=-buildid= -trimpath -o controller ./cmd/main.go

# ---
# Needs ubuntu for installing git/openssh
FROM debian:bullseye-slim

RUN apt-get -y update && apt-get -y install ca-certificates && update-ca-certificates && apt-get -y install openssh-client git

# Create appusergroup and appuser
ENV GROUP=appusergroup
ENV GUID=10002

RUN addgroup --gid "${GUID}" "$GROUP"

ENV USER=appuser
ENV UID=10001

# See https://stackoverflow.com/a/55757473/12429735RUN
# and https://medium.com/@chemidy/create-the-smallest-and-secured-golang-docker-image-based-on-scratch-4752223b7324
RUN adduser \
--disabled-password \
--gecos "" \
--home "/nonexistent" \
--shell "/sbin/nologin" \
--no-create-home \
--uid "${UID}" \
--gid "${GUID}" \
"$USER"

# Disabled it must be checked if this allows end users to provide additional custom ca certs
# RUN chmod g+w /etc/ssl/certs/ca-certificates.crt && chgrp ${GROUP} /etc/ssl/certs/ca-certificates.crt

USER ${USER}

# Name it kapp-controller to identify it easier in process tree
COPY --from=0 /go/src/github.com/vmware-tanzu/carvel-kapp-controller/controller kapp-controller

# fetchers
COPY --from=0 /helm-unpacked/linux-amd64/helm .
COPY --from=0 /usr/local/bin/imgpkg .
COPY --from=0 /usr/local/bin/vendir .

# templaters
COPY --from=0 /usr/local/bin/ytt .
COPY --from=0 /usr/local/bin/kbld .
COPY --from=0 /usr/local/bin/sops .

# deployers
COPY --from=0 /usr/local/bin/kapp .

ENV PATH="/:${PATH}"
ENTRYPOINT ["/kapp-controller"]
