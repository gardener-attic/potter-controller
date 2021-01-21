# Build the manager binary
FROM eu.gcr.io/gardener-project/3rd/golang:1.15.5 as builder

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY pkg/ pkg/

# Build
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager main.go

FROM eu.gcr.io/gardenlinux/gardenlinux:184.0

RUN apt-get -y update && apt-get -y install ca-certificates && update-ca-certificates

# Disable start of Berkeley DB
# copied installation package files from https://github.wdf.sap.corp/devx-wing/noberkeley/wiki/NoBerkeley-Packages
COPY noberkeley/noberkeley_1.0.0-3_amd64.deb .
COPY noberkeley/noberkeley-dev_1.0.0-3_amd64.deb .
RUN apt-get -y install ./noberkeley_1.0.0-3_amd64.deb ./noberkeley-dev_1.0.0-3_amd64.deb && \
    rm noberkeley_1.0.0-3_amd64.deb && \
    rm noberkeley-dev_1.0.0-3_amd64.deb

WORKDIR /

# Create appuser
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
"$USER"

COPY --from=builder /workspace/manager .

USER ${USER}:${USER}

ENTRYPOINT ["/manager"]
