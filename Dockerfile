# build frontend
FROM node:22 as fe
WORKDIR /src
COPY .git .git/
COPY frontend ./frontend
COPY scripts.sh .
RUN ./scripts.sh build-frontend

# build backend
FROM golang:1.23 as be
WORKDIR /src
COPY . ./
COPY --from=fe /src/frontend/build ./frontend/build/
RUN ./scripts.sh build-backend

# deploy
FROM debian:12
LABEL org.opencontainers.image.source="https://github.com/0x2E/fusion"
RUN apt-get update && apt-get install -y sqlite3 ca-certificates
WORKDIR /fusion
COPY --from=be /src/build/fusion ./
EXPOSE 8080
RUN mkdir /data
ENV DB="/data/fusion.db"
CMD [ "./fusion" ]
