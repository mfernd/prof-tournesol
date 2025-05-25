FROM python:3.12.3

RUN python -m pip install --upgrade pip

WORKDIR /code

COPY ./requirements.txt /code/requirements.txt

RUN pip install --no-cache-dir --upgrade -r /code/requirements.txt

COPY . /code

EXPOSE 8080

CMD ["fastapi", "run", "main.py", "--port", "8080"]