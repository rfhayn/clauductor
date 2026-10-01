export interface Statement {
  id: string;
  accountId: string;
  month: string;
  pdf: Buffer;
}

export declare function listStatements(accountId: string): Promise<Omit<Statement, "pdf">[]>;
export declare function loadStatement(id: string): Promise<Statement | undefined>;
