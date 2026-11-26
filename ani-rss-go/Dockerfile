# 需要 BuildKit，构建上下文为仓库根目录：
#   docker buildx build -f ani-rss-go/Dockerfile --build-arg VERSION=1.0.0 \
#     --build-arg COMMIT=$(git rev-parse HEAD) --build-arg BRANCH=$(git rev-parse --abbrev-ref HEAD) \
#     -t ani-rss-go --load .
# 国内网络可追加 --build-arg GOPROXY=https://goproxy.cn,direct

FROM --platform=$BUILDPLATFORM node:26-alpine AS ui
RUN npm install -g pnpm@11.9.0
WORKDIR /src/ani-rss-ui
COPY ani-rss-ui/ ./
RUN pnpm install --no-frozen-lockfile && pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27.4-alpine AS build
ARG GOPROXY
WORKDIR /src
COPY ani-rss-go/go.mod ani-rss-go/go.sum ./
RUN go mod download
COPY ani-rss-go/ ./
COPY --from=ui /src/ani-rss-ui/dist/ ./internal/web/dist/
ARG TARGETOS TARGETARCH TARGETVARIANT
ARG VERSION=dev COMMIT= BRANCH=
RUN PKG=ani-rss-go/internal/buildinfo && \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -trimpath \
      -ldflags "-s -w -X $PKG.Version=$VERSION -X $PKG.Commit=$COMMIT -X $PKG.Branch=$BRANCH" \
      -o /out/ani-rss .

FROM alpine:3.22
RUN apk add --no-cache ca-certificates tzdata su-exec
COPY --from=build /out/ani-rss /usr/app/ani-rss
COPY ani-rss-go/packaging/run.sh ani-rss-go/packaging/exec.sh /usr/app/
RUN sed -i 's/\r//g' /usr/app/*.sh && chmod +x /usr/app/*.sh
WORKDIR /usr/app
VOLUME /config
ENV PUID=0 PGID=0 UMASK=022
ENV PORT=7789 CONFIG=/config TZ=Asia/Shanghai
EXPOSE $PORT
CMD ["/usr/app/exec.sh"]
