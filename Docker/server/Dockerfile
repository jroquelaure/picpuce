FROM ubuntu:5001/debian:latest

RUN mkdir /app
WORKDIR /app
ADD /picpuce-server/scenarioDesc.json /app/scenarioDesc.json
ADD /picpuce-server/picpuce-server /app/picpuce-server

CMD ["./picpuce-server"]