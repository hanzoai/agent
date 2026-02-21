import { useMemo, useState } from "react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Bot, ArrowLeft, ChevronRight, Search, Tag, TrendingUp } from "@/components/ui/icon-bridge";

type MarketBot = {
  id: string;
  name: string;
  category: string;
  description: string;
  pricing: string;
  builder: string;
  score: number;
};

const MARKET_BOTS: MarketBot[] = [
  {
    id: "sales-closer",
    name: "Sales Closer",
    category: "Revenue",
    description: "Turns inbound leads into booked calls with personalized outreach.",
    pricing: "$29/mo + usage",
    builder: "Studio Nova",
    score: 97,
  },
  {
    id: "support-deflector",
    name: "Support Deflector",
    category: "Support",
    description: "Resolves repetitive support tickets before they hit your human queue.",
    pricing: "$19/mo + usage",
    builder: "HelpLabs",
    score: 94,
  },
  {
    id: "research-scout",
    name: "Research Scout",
    category: "Research",
    description: "Runs fast company and market research with citations and summaries.",
    pricing: "$39/mo + usage",
    builder: "Signal Team",
    score: 95,
  },
];

export function MarketPage() {
  const [query, setQuery] = useState("");
  const [index, setIndex] = useState(0);

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase();
    if (!q) return MARKET_BOTS;
    return MARKET_BOTS.filter((bot) =>
      `${bot.name} ${bot.category} ${bot.description} ${bot.builder}`.toLowerCase().includes(q)
    );
  }, [query]);

  const current = filtered[index] ?? null;

  const next = () => {
    if (!filtered.length) return;
    setIndex((prev) => (prev + 1) % filtered.length);
  };

  const prev = () => {
    if (!filtered.length) return;
    setIndex((prev) => (prev - 1 + filtered.length) % filtered.length);
  };

  return (
    <div className="mx-auto w-full max-w-5xl space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-2xl font-semibold tracking-tight">Bot Market</h1>
          <p className="text-sm text-muted-foreground">
            Swipe, test, and deploy intelligence. List your bots and get paid.
          </p>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" asChild>
            <a href="https://flow.hanzo.ai" target="_blank" rel="noopener noreferrer">
              Open Builder
            </a>
          </Button>
          <Button>List Your Bot</Button>
        </div>
      </div>

      <Card>
        <CardHeader className="pb-3">
          <CardTitle className="text-base">Discover</CardTitle>
        </CardHeader>
        <CardContent>
          <div className="relative">
            <Search size={14} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-muted-foreground" />
            <Input
              className="pl-8"
              placeholder="Search bots by category, capability, or builder..."
              value={query}
              onChange={(e) => {
                setQuery(e.target.value);
                setIndex(0);
              }}
            />
          </div>
        </CardContent>
      </Card>

      <div className="grid gap-6 lg:grid-cols-[1fr_320px]">
        <Card className="min-h-[360px]">
          <CardContent className="flex h-full flex-col justify-between p-6">
            {current ? (
              <>
                <div className="space-y-3">
                  <div className="flex items-start justify-between gap-2">
                    <div>
                      <h2 className="text-xl font-semibold">{current.name}</h2>
                      <p className="text-sm text-muted-foreground">by {current.builder}</p>
                    </div>
                    <Badge variant="secondary">{current.category}</Badge>
                  </div>
                  <p className="text-sm">{current.description}</p>
                </div>

                <div className="space-y-3">
                  <div className="flex items-center justify-between rounded-md border border-border/50 bg-muted/20 p-3">
                    <span className="text-sm text-muted-foreground">Pricing</span>
                    <span className="text-sm font-medium">{current.pricing}</span>
                  </div>
                  <div className="flex items-center justify-between rounded-md border border-border/50 bg-muted/20 p-3">
                    <span className="text-sm text-muted-foreground">Fit Score</span>
                    <span className="text-sm font-medium">{current.score}/100</span>
                  </div>
                </div>

                <div className="flex flex-wrap items-center gap-2">
                  <Button variant="outline" onClick={prev}>
                    <ArrowLeft size={14} />
                    Skip
                  </Button>
                  <Button onClick={next}>
                    <ChevronRight size={14} />
                    Next
                  </Button>
                  <Button variant="secondary">Try On My Data</Button>
                  <Button>Deploy Bot</Button>
                </div>
              </>
            ) : (
              <div className="flex h-full flex-col items-center justify-center gap-2 text-center">
                <Bot size={26} className="text-muted-foreground" />
                <p className="text-sm text-muted-foreground">No bots found for that filter.</p>
              </div>
            )}
          </CardContent>
        </Card>

        <div className="space-y-4">
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">Referral & Affiliate</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <p className="text-muted-foreground">Share your referral link and earn rewards on usage.</p>
              <div className="rounded-md border border-border/50 bg-muted/20 p-2 font-mono text-xs">
                https://hanzo.bot/r/your-code
              </div>
              <Button className="w-full" variant="outline">Copy Referral Link</Button>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base">Rewards Snapshot</CardTitle>
            </CardHeader>
            <CardContent className="space-y-2 text-sm">
              <div className="flex items-center justify-between">
                <span className="inline-flex items-center gap-1 text-muted-foreground"><TrendingUp size={13} /> This Month</span>
                <span className="font-medium">$0.00</span>
              </div>
              <div className="flex items-center justify-between">
                <span className="inline-flex items-center gap-1 text-muted-foreground"><Tag size={13} /> Active Referrals</span>
                <span className="font-medium">0</span>
              </div>
              <Button className="w-full">Open Payout Wallet</Button>
            </CardContent>
          </Card>
        </div>
      </div>
    </div>
  );
}
