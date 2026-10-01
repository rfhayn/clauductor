import { Router, Request, Response } from "express";
import { listStatements, loadStatement } from "../db";

export const router = Router();

/** 404 unless the signed-in user owns accountId: another account's data is never confirmed to exist. */
function requireAccount(req: Request, res: Response, accountId: string): boolean {
  if (req.user?.accountId !== accountId) {
    res.status(404).end();
    return false;
  }
  return true;
}

router.get("/accounts/:accountId/statements", async (req, res) => {
  if (!requireAccount(req, res, req.params.accountId)) return;
  res.json(await listStatements(req.params.accountId));
});

router.get("/accounts/:accountId/statements/:id.pdf", async (req, res) => {
  const statement = await loadStatement(req.params.id);
  if (!statement) {
    res.status(404).end();
    return;
  }
  res.type("application/pdf").send(statement.pdf);
});
