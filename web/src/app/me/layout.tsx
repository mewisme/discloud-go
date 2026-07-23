import { MeNav } from "@/components/me-nav";
import { PageBreadcrumb } from "@/components/page-breadcrumb";

export default function MeLayout({ children }: { children: React.ReactNode }) {
  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6">
      <PageBreadcrumb items={[{ label: "Account" }]} />
      <MeNav />
      {children}
    </div>
  );
}
