FROM alpine:latest

RUN apk update
RUN apk --no-cache --update add ca-certificates

ADD streaming_grpc app
ADD creds.json /creds.json

ENV PORT 8080
ENV GOOGLE_APPLICATION_CREDENTIALS /creds.json

CMD ["./app"]
