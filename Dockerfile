FROM golang:1.23.5-bookworm

WORKDIR /app
COPY . .
RUN go build  -o nginx-reloader .

CMD /app/nginx-reloader


