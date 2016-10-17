FROM ubuntu:14.04

# Install TraceView packages and agent.
RUN apt-get update && apt-get -y install wget
ARG APPNETA_KEY
RUN wget https://files.appneta.com/install_appneta.sh
RUN sh ./install_appneta.sh $APPNETA_KEY

# Install Node.js
RUN apt-get -y install build-essential nodejs npm
RUN ln -s /usr/bin/nodejs /usr/bin/node

# Install Node app
RUN mkdir -p /nodejs
COPY app.js /nodejs
WORKDIR /nodejs
RUN npm install --save traceview

# Script to run before testing to start services such as tracelyzer and app
ADD start_services.sh /start_services.sh
EXPOSE 8082
CMD [ "bash", "/start_services.sh" ]
