from pydantic import BaseModel

class Text(BaseModel):
    prompt: str
