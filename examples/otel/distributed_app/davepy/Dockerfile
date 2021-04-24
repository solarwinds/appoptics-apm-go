FROM ubuntu:16.04

# Install uWSGI and instrumentation
RUN apt-get update && apt-get -y install python-pip python-dev build-essential
RUN pip install appoptics
RUN pip install uwsgi flask

# Script to run before testing to start services such as tracelyzer and apache
ADD start_services.sh /start_services.sh

# uWSGI stack
ADD app /home/app/

EXPOSE 8083
CMD [ "bash", "/start_services.sh" ]

