# picpuce

Goal : stress a bit artifactory with different load profile (parallelism, variance on artifacts sizes, nb of files,...) and measure behavior. Have fun with go and micro service architecture

a simple client-server microservice app that does the following

- loads a scenario desription
    - create nbThreads scenarios
    - foreach scenarios creates nbFiles files (built with x random bytes where minSize < x < maxSize)
    - transfers all generated files (streams chunks) to the runner service
    - Once all scenarios loaded, triggers Run scenarios in parallel (runner service will run nbThreads scenario in parrallel)


Concept in uses :

- multithreading
- streams
- protobuf
- grpc and go-micro
- http and http tracing



