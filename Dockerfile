FROM golang:1.11.5-alpine3.9 as builder
RUN mkdir /app
ADD . /app/
WORKDIR /app/
RUN apk add --no-cache gcc libc-dev git && \
    go build -o /app/cloudflare-kube-dns .


FROM alpine:3.9
COPY --from=builder /app/cloudflare-kube-dns /app/
WORKDIR /app
RUN apk add --no-cache ca-certificates
ENV CF_API_KEY=""
ENV CF_API_MAIL=""
ENV CF_API_DOMAIN=""

USER nobody
ENTRYPOINT ["/app/cloudflare-kube-dns"]