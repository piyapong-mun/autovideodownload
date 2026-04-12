# 1. Start with a Python environment
FROM golang:1.25.6

# 2. Set the folder for our app
WORKDIR /app

# 3. Copy our code into that folder
COPY . .

# 4. Init go mod
RUN go mod init github.com/piyapong-mun/autovideodownload

# 5. Install dependencies
RUN go mod tidy

# Expose port
EXPOSE 1112

# 5. Tell the container how to start the app
CMD ["go", "run", "main.go"]

