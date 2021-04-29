FROM ubuntu:17.10

RUN apt-get update
RUN apt-get -y install build-essential python-dev python-pip

# Install Node app
RUN mkdir -p /carolpy
COPY app.py /carolpy
COPY requirements.txt /carolpy
WORKDIR /carolpy
RUN pip install -r requirements.txt

# Script to run before testing to start services such as tracelyzer and app
ADD start_services.sh /start_services.sh
EXPOSE 8082
CMD [ "bash", "/start_services.sh" ]
