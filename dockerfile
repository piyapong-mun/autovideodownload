# 1. Start with a Python environment
FROM golang:1.25.6

# 2. Set the folder for our app
WORKDIR /app

# 3. Copy our code into that folder
COPY . .

# 4. Install dependencies
RUN go mod tidy

# 5. Tell the container how to start the app
CMD ["go", "run", "main.go"]