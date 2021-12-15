# SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

#### BUILDER ####
FROM eu.gcr.io/gardener-project/3rd/golang:1.16.11 AS builder

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
FROM eu.gcr.io/gardenlinux/gardenlinux:590.0-276f22-amd64-base-slim AS base

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get --yes -o Dpkg::Options::="--force-confnew" install ca-certificates \
    && rm -rf /var/lib/apt /var/cache/apt

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

USER ${USER}:${USER}

#### Helm Deployer Controller ####
FROM base as landscaper-controller

COPY --from=builder /go/bin/landscaper-controller /landscaper-controller

WORKDIR /

ENTRYPOINT ["/landscaper-controller"]