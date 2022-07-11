# SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

#### BUILDER ####
FROM eu.gcr.io/gardener-project/3rd/golang:1.17.9 AS builder

# Commit hash of version we use, please crosscheck go.mod
ARG landscaper_commit_hash="c077da8895eae68100137e63ab466708dae0aa17"

WORKDIR /go/src/github.com/gardener

RUN apt-get -y install git
RUN git clone https://github.com/gardener/landscaper.git
WORKDIR /go/src/github.com/gardener/landscaper
RUN git checkout ${landscaper_commit_hash}

ARG EFFECTIVE_VERSION

RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

#### BASE ####
FROM gcr.io/distroless/static-debian11:nonroot AS base

#### Helm Deployer Controller ####
FROM base as landscaper-controller

COPY --from=builder /go/bin/landscaper-controller /landscaper-controller

WORKDIR /

ENTRYPOINT ["/landscaper-controller"]