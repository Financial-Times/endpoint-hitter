FROM golang:1

COPY . /
WORKDIR /

RUN go build -o /endpoint-hitter

COPY uuids.txt /

CMD /endpoint-hitter
