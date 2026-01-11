# build backend
FROM golang:1.24 AS be
# Add Arguments for target OS and architecture (provided by buildx)
ARG TARGETOS
ARG TARGETARCH
WORKDIR /src
COPY . ./
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build \
  -ldflags '-extldflags "-static"' \
  -o ./build/fusion \
  ./cmd/server

# deploy
FROM alpine:3.21.0
LABEL org.opencontainers.image.source="https://github.com/0x2E/fusion"
WORKDIR /fusion
COPY --from=be /src/build/fusion ./
EXPOSE 8080
RUN mkdir /data
ENV DB="/data/fusion.db"
CMD [ "./fusion" ]
