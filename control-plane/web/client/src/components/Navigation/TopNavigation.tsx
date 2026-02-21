import React from "react";
import { useLocation, Link } from "react-router-dom";
import { cn } from "@/lib/utils";
import {
  Breadcrumb,
  BreadcrumbList,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "@/components/ui/breadcrumb";
import { SidebarTrigger } from "@/components/ui/sidebar";
import { Separator } from "@/components/ui/separator";
import { ModeToggle } from "@/components/ui/mode-toggle";
import { SpaceTabs } from "@/components/spaces/SpaceTabs";
import { Search } from "@/components/ui/icon-bridge";

interface TopNavigationProps {
  onCommandPalette?: () => void;
}

export function TopNavigation({ onCommandPalette }: TopNavigationProps) {
  const location = useLocation();
  const isPlayground = location.pathname === "/playground" || location.pathname.startsWith("/playground");

  const generateBreadcrumbs = () => {
    const pathSegments = location.pathname.split("/").filter(Boolean);
    const breadcrumbs = [{ label: "Home", href: "/" }];

    const routeMappings: Record<
      string,
      { label: string; href: string; parent?: string }
    > = {
      reasoners: { label: "Bots", href: "/reasoners/all" },
      "reasoners/all": {
        label: "All Bots",
        href: "/reasoners/all",
        parent: "reasoners",
      },
      nodes: { label: "Nodes", href: "/nodes" },
      packages: { label: "Packages", href: "/packages" },
      settings: { label: "Settings", href: "/settings" },
      playground: { label: "Playground", href: "/playground" },
      agents: { label: "Playground", href: "/playground" },
      canvas: { label: "Playground", href: "/playground" },
      market: { label: "Marketplace", href: "/market" },
      dashboard: { label: "Dashboard", href: "/dashboard" },
      identity: { label: "Identity", href: "/identity/dids" },
      "identity/dids": { label: "DID Explorer", href: "/identity/dids" },
      "identity/credentials": { label: "Credentials", href: "/identity/credentials" },
    };

    let currentPath = "";
    pathSegments.forEach((segment, index) => {
      currentPath += `/${segment}`;
      const routeKey = pathSegments.slice(0, index + 1).join("/");

      if (routeMappings[routeKey]) {
        const mapping = routeMappings[routeKey];

        if (routeKey === "reasoners/all") {
          const existingIndex = breadcrumbs.findIndex((b) => b.label === "Bots");
          if (existingIndex !== -1) {
            breadcrumbs[existingIndex] = { label: "Bots", href: "/reasoners/all" };
          } else {
            breadcrumbs.push({ label: "Bots", href: "/reasoners/all" });
          }
          return;
        }

        breadcrumbs.push({ label: mapping.label, href: mapping.href });
      } else {
        let label = segment.charAt(0).toUpperCase() + segment.slice(1).replace("-", " ");
        const href = currentPath;

        if (pathSegments[index - 1] === "reasoners" && segment !== "all") {
          try {
            const decodedId = decodeURIComponent(segment);
            const parts = decodedId.split(".");
            label = parts.length >= 2 ? parts[parts.length - 1] : decodedId;
          } catch {
            label = segment;
          }
          const botsIndex = breadcrumbs.findIndex((b) => b.label === "Bots");
          if (botsIndex !== -1) breadcrumbs[botsIndex].href = "/reasoners/all";
        } else if (pathSegments[index - 1] === "nodes") {
          label = `Node ${segment}`;
        }

        breadcrumbs.push({ label, href });
      }
    });

    return breadcrumbs;
  };

  const breadcrumbs = generateBreadcrumbs();

  return (
    <header
      className={cn(
        "flex flex-col sticky top-0 z-50",
        "bg-gradient-to-r from-bg-base via-bg-subtle to-bg-base",
        "backdrop-blur-xl border-none",
        "shadow-soft transition-all duration-200",
      )}
    >
      {/* Main nav row */}
      <div className="h-12 flex items-center justify-between px-4 md:px-6 lg:px-8">
        {/* Left: sidebar trigger + breadcrumbs */}
        <div className="flex items-center gap-3 flex-1 min-w-0">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="h-4" />
          <Breadcrumb>
            <BreadcrumbList>
              {breadcrumbs.map((crumb, index) => {
                const isFirst = index === 0;
                const isLast = index === breadcrumbs.length - 1;
                const isHiddenOnMobile = !isFirst && !isLast;
                return (
                  <React.Fragment key={crumb.href}>
                    <BreadcrumbItem className={cn(isHiddenOnMobile && "hidden md:inline-flex")}>
                      {isLast ? (
                        <BreadcrumbPage className="max-w-[150px] md:max-w-[200px] truncate" title={crumb.label}>
                          {crumb.label}
                        </BreadcrumbPage>
                      ) : (
                        <BreadcrumbLink asChild>
                          <Link to={crumb.href} className="max-w-[150px] md:max-w-[200px] truncate" title={crumb.label}>
                            {crumb.label}
                          </Link>
                        </BreadcrumbLink>
                      )}
                    </BreadcrumbItem>
                    {index < breadcrumbs.length - 1 && (
                      <BreadcrumbSeparator className={cn(isHiddenOnMobile && "hidden md:list-item")} />
                    )}
                  </React.Fragment>
                );
              })}
            </BreadcrumbList>
          </Breadcrumb>
        </div>

        {/* Right: Cmd+K + theme */}
        <div className="flex items-center gap-2">
          {onCommandPalette && (
            <button
              onClick={onCommandPalette}
              className={cn(
                "flex items-center gap-2 rounded-lg border border-border/50 bg-muted/30 px-3 py-1.5",
                "text-xs text-muted-foreground hover:bg-muted/50 hover:text-foreground transition-colors",
              )}
            >
              <Search size={13} />
              <span className="hidden sm:inline">Search...</span>
              <kbd className="hidden sm:inline-flex h-5 items-center gap-0.5 rounded border border-border/60 bg-muted/40 px-1.5 font-mono text-[10px] text-muted-foreground">
                <span className="text-[11px]">⌘</span>K
              </kbd>
            </button>
          )}
          <ModeToggle />
        </div>
      </div>

      {/* Space tabs row (only on playground) */}
      {isPlayground && <SpaceTabs />}
    </header>
  );
}
