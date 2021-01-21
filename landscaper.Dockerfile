# SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

#### BUILDER ####
FROM eu.gcr.io/gardener-project/3rd/golang:1.15.5 AS builder

# Commit hash of version we use, please crosscheck go.mod
ARG landscaper_commit_hash="c6c3f267bd5f3af60d79fa7adae7ddf70b405dfc"

WORKDIR /go/src/github.com/gardener

RUN apt-get -y install git
RUN git clone https://github.com/gardener/landscaper.git
WORKDIR /go/src/github.com/gardener/landscaper
RUN git checkout ${landscaper_commit_hash}

ARG EFFECTIVE_VERSION

RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

#### BASE ####
FROM eu.gcr.io/gardenlinux/gardenlinux:184.0 AS base

RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get --yes -o Dpkg::Options::="--force-confnew" install ca-certificates \
    && rm -rf /var/lib/apt /var/cache/apt

# Disable start of Berkeley DB
# copied installation package files from https://github.wdf.sap.corp/devx-wing/noberkeley/wiki/NoBerkeley-Packages
COPY noberkeley/noberkeley_1.0.0-3_amd64.deb .
COPY noberkeley/noberkeley-dev_1.0.0-3_amd64.deb .
RUN apt-get -y install ./noberkeley_1.0.0-3_amd64.deb ./noberkeley-dev_1.0.0-3_amd64.deb && \
    rm noberkeley_1.0.0-3_amd64.deb && \
    rm noberkeley-dev_1.0.0-3_amd64.deb

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

# Fixes vulnerabilities
# CVE-2020-29361, CVE-2020-29362, and CVE-2020-29363
# CVE-2019-20838
# CVE-2019-9923
# RUN apt-get update && apt-get -y upgrade p11-kit=0.23.22 \
    # pcre=8.44 \
    # tar=1.32
# triaged by bot_871@protecode-sc.local (GCR)
# TODO: packages for gardenlinux/debian (bullseye) not available yet

# Disable start of Berkeley DB
# copied installation package files from https://github.wdf.sap.corp/devx-wing/noberkeley/wiki/NoBerkeley-Packages
COPY noberkeley/noberkeley_1.0.0-3_amd64.deb .
COPY noberkeley/noberkeley-dev_1.0.0-3_amd64.deb .
RUN apt-get -y install ./noberkeley_1.0.0-3_amd64.deb ./noberkeley-dev_1.0.0-3_amd64.deb && \
    rm noberkeley_1.0.0-3_amd64.deb && \
    rm noberkeley-dev_1.0.0-3_amd64.deb
RUN apt-get -y --purge remove db5.3

USER ${USER}:${USER}

#### Helm Deployer Controller ####
FROM base as landscaper-controller

COPY --from=builder /go/bin/landscaper-controller /landscaper-controller

WORKDIR /

ENTRYPOINT ["/landscaper-controller"]