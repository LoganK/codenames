FROM golang:1.12-stretch as builder

# Install npm and parcel
RUN curl -sL https://deb.nodesource.com/setup_12.x | bash - && \
    apt-get install -y nodejs && \
    apt-get clean && \
    npm install -g parcel-bundler
    
# Copy project into docker instance
WORKDIR /go/src/github.com/jbowens/codenames
COPY . .

# Get the go app
RUN go get -u github.com/jbowens/codenames

# Build backend and frontend 
RUN CGO_ENABLED=0 GOOS=linux go build cmd/codenames/main.go && \
    cd frontend/ && \
    npm install && \
    sh build.sh


FROM alpine:latest

WORKDIR /app
COPY --from=builder /go/src/github.com/jbowens/codenames/codenames .
COPY --from=builder /go/src/github.com/jbowens/codenames/frontend/*.tmpl frontend/
COPY --from=builder /go/src/github.com/jbowens/codenames/frontend/dist frontend/dist/
COPY --from=builder /go/src/github.com/jbowens/codenames/assets assets/

# Expose 9091 port
EXPOSE 9091/tcp

# Set entrypoint command
CMD ["/app/main"]
