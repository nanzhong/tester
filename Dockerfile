FROM golang:1.14.6 AS build
COPY . /tester
WORKDIR /tester
RUN make build

FROM golang:1.14.6
COPY --from=build /tester/dist/tester-linux-amd64 /bin/tester