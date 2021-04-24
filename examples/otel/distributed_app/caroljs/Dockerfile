FROM ubuntu:16.04

# Install Node.js
RUN apt-get update && apt-get -y install build-essential nodejs npm
RUN ln -s /usr/bin/nodejs /usr/bin/node

# Install Node app
RUN mkdir -p /nodejs
COPY app.js /nodejs
WORKDIR /nodejs
RUN npm install --save appoptics

# Script to run before testing to start services such as tracelyzer and app
ADD start_services.sh /start_services.sh
EXPOSE 8082
CMD [ "bash", "/start_services.sh" ]
