FROM node:20-alpine
WORKDIR /app

# Cache npm install as a separate layer
COPY package*.json ./
RUN npm install

# Source is bind-mounted at runtime
COPY . .

EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
