FROM ubuntu:14.04

# Install AppNeta packages and agent.
RUN apt-get update && apt-get -y install wget
ARG APPNETA_KEY
RUN wget https://files.appneta.com/install_appneta.sh
RUN sh ./install_appneta.sh $APPNETA_KEY

# Install uWSGI and instrumentation
RUN apt-get -y install python-pip python-dev build-essential
RUN pip install oboe
RUN pip install uwsgi flask

# Script to run before testing to start services such as tracelyzer and apache
ADD start_services.sh /start_services.sh

# uWSGI stack
ADD app /home/app/

EXPOSE 8083
CMD [ "bash", "/start_services.sh" ]

