import type { Metadata } from "next";

import { AuthForm } from "@/components/auth-form";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export const metadata: Metadata = { title: "Sign up" };

export default function SignUpPage() {
  return (
    <div className="flex flex-col gap-6">
      <PageBreadcrumb items={[{ label: "Sign up" }]} />
      <AuthForm mode="signup" />
    </div>
  );
}
