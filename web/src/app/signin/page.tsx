import type { Metadata } from "next";

import { AuthForm } from "@/components/auth-form";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const metadata: Metadata = { title: "Sign in" };

export default function SignInPage() {
  return (
    <div className="flex flex-col gap-6">
      <PageBreadcrumb items={[{ label: "Sign in" }]} />
      <AuthForm mode="signin" />
    </div>
  );
}
