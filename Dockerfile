FROM golang:1.14.5-alpine as builder

# Force Go to use the cgo based DNS resolver. This is required to ensure DNS
# queries required to connect to linked containers succeed.
ENV GODEBUG netdns=cgo

# Pass a tag, branch or a commit using build-arg.  This allows a docker
# image to be built from a specified Git state.  The default image
# will use the Git tip of master by default.
ARG checkout="master"

# Install dependencies and build the binaries.
RUN apk add --no-cache --update alpine-sdk \
    git \
    make \
    gcc \
&&  git clone https://github.com/cryptomeow/lnd /go/src/github.com/cryptomeow/lnd \
&&  cd /go/src/github.com/cryptomeow/lnd \
&&  git checkout $checkout \
&&  make \
&&  make install tags="signrpc walletrpc chainrpc invoicesrpc"

# Start a new, final image.
FROM alpine as final

# Define a root volume for data persistence.
VOLUME /root/.lnd

# Add bash, jq and ca-certs, for quality of life and SSL-related reasons.
RUN apk --no-cache add \
    bash \
    jq \
    ca-certificates

# Copy the binaries from the builder image.
COPY --from=builder /go/bin/lncli /bin/
COPY --from=builder /go/bin/lnd /bin/

# Expose lnd ports (p2p, rpc).
EXPOSE 9735 10009

# Specify the start command and entrypoint as the lnd daemon.
ENTRYPOINT ["lnd"]
