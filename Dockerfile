FROM golang:1.24-alpine

WORKDIR /usr/src/app

COPY go.mod go.sum ./
RUN go mod download && go mod verify

COPY . .
RUN go build -v -o /usr/local/bin/unquestionify-relay .

EXPOSE 80/tcp
EXPOSE 443/tcp

CMD /usr/local/bin/unquestionify-relay
