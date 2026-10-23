"use client";

import { useState } from "react";
import { Trash2 } from "lucide-react";
import { toast } from "sonner";

import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

/**
 * "Delete my account": deactivates the account after an emailed confirmation
 * code. Data is kept server-side; every sign-in is blocked afterwards.
 */
export function DeleteAccountDialog() {
  const { t } = useLocale();
  const { logout } = useAuth();
  const [open, setOpen] = useState(false);
  const [codeSent, setCodeSent] = useState(false);
  const [code, setCode] = useState("");
  const [pending, setPending] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const reset = () => {
    setCodeSent(false);
    setCode("");
    setError(null);
    setPending(false);
  };

  const errorMessage = (err: unknown) => {
    if (isApiError(err)) {
      if (err.code === "INVALID_EMAIL_CODE")
        return t("auth.email_code.invalid");
      if (err.code === "RATE_LIMITED") return t("auth.login.rate_limited");
      return err.message || t("auth.delete_account.error");
    }
    return t("auth.delete_account.error");
  };

  const sendCode = async () => {
    setError(null);
    setPending(true);
    try {
      await authService.requestAccountDeactivationCode();
      setCodeSent(true);
    } catch (err) {
      setError(errorMessage(err));
    } finally {
      setPending(false);
    }
  };

  const confirm = async () => {
    if (!/^\d{6,}$/.test(code.trim())) {
      setError(t("auth.email_code.validation.code"));
      return;
    }
    setError(null);
    setPending(true);
    try {
      await authService.deactivateAccount(code.trim());
      toast.success(t("auth.delete_account.success"));
      try {
        await logout();
      } catch {
        // Session cookie is already cleared by the BFF.
      }
      window.location.replace(routes.guest.login);
    } catch (err) {
      setError(errorMessage(err));
      setPending(false);
    }
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) reset();
      }}
    >
      <DialogTrigger asChild>
        <Button type="button" variant="destructive">
          <Trash2 aria-hidden />
          {t("auth.delete_account.button")}
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("auth.delete_account.dialog_title")}</DialogTitle>
          <DialogDescription>
            {t("auth.delete_account.dialog_description")}
          </DialogDescription>
        </DialogHeader>

        {error ? (
          <Alert variant="destructive">
            <AlertDescription>{error}</AlertDescription>
          </Alert>
        ) : null}

        {codeSent ? (
          <div className="space-y-2">
            <Label htmlFor="delete-account-code">
              {t("auth.email_code.code")}
            </Label>
            <Input
              id="delete-account-code"
              value={code}
              onChange={(event) => setCode(event.target.value)}
              autoComplete="one-time-code"
              inputMode="numeric"
              maxLength={6}
              placeholder={t("auth.email_code.code_placeholder")}
            />
            <p className="text-muted-foreground text-sm">
              {t("auth.delete_account.code_sent")}
            </p>
          </div>
        ) : null}

        <DialogFooter>
          <Button
            type="button"
            variant="outline"
            disabled={pending}
            onClick={() => setOpen(false)}
          >
            {t("common.cancel")}
          </Button>
          {codeSent ? (
            <Button
              type="button"
              variant="destructive"
              disabled={pending}
              onClick={confirm}
            >
              {pending
                ? t("auth.delete_account.deleting")
                : t("auth.delete_account.confirm")}
            </Button>
          ) : (
            <Button type="button" disabled={pending} onClick={sendCode}>
              {pending
                ? t("auth.email_code.sending")
                : t("auth.delete_account.send_code")}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
