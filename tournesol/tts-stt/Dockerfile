FROM python:3.12.3-slim

# Install system dependencies & update pip
RUN apt-get update && apt-get install -y ffmpeg && apt-get clean
RUN python -m pip install --upgrade pip

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

COPY . /code

RUN mkdir -p /code/files
RUN mkdir -p /code/files/sent_audios
RUN mkdir -p /code/files/received_audios

EXPOSE 8080

CMD ["fastapi", "run", "main.py", "--port", "8080"]