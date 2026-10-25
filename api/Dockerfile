# Build context is the repository root, not api/.
# In Railway, leave the service root directory as the repo root.
# railway.json points at this file.
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o /out/gutsaga-api ./api

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/gutsaga-api /gutsaga-api
ENV PORT=8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/gutsaga-api"]
