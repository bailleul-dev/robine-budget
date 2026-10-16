from pydantic import BaseModel
from .sync import cmd_accounts
from .config import Credentials
from fastapi import FastAPI

app = FastAPI()


class SyncRequest(BaseModel):
    login: str
    password: str
    session: str


@app.post("/sync/{creds_id}")
async def sync(creds_id: str, body: SyncRequest):
    creds = Credentials(body.login, body.password)
    result = cmd_accounts(creds_id, creds, body.session)

    return result
