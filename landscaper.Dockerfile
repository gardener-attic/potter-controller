# SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
#
# SPDX-License-Identifier: Apache-2.0

# 0.0.2 landscaper controller commit 2ed391ada37cefb140361df561ba22ce43a29c3a (Nov 24, 2020)

#### BUILDER ####
FROM eu.gcr.io/gardener-project/3rd/golang:1.15.5 AS builder

WORKDIR /go/src/github.com/gardener/landscaper
COPY . .

ARG EFFECTIVE_VERSION

RUN make install EFFECTIVE_VERSION=$EFFECTIVE_VERSION

#### BASE ####
FROM eu.gcr.io/gardenlinux/gardenlinux:184.0 AS base

#### Helm Deployer Controller ####
FROM base as landscaper-controller

COPY --from=builder /go/bin/landscaper-controller /landscaper-controller

WORKDIR /

ENTRYPOINT ["/landscaper-controller"]