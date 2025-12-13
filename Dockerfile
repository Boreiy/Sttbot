FROM alpine:latest

WORKDIR /app

# Copy the pre-built binary
COPY server ./

# Copy migrations
COPY migrations ./migrations

EXPOSE 2010

CMD ["./server"]
