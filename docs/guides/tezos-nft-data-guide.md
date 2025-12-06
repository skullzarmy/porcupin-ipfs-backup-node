# Tezos NFT Data Fetching Guide

A practical guide for fetching NFT data from the Tezos blockchain using TZKT and objkt APIs. Written based on building Porcupin and applicable to projects like "2025 Wrapped" analytics.

---

## Table of Contents

1. [API Overview](#api-overview)
2. [TZKT API](#tzkt-api)
    - [Authentication](#authentication)
    - [Core Endpoints](#core-endpoints)
    - [Fetching Owned NFTs](#fetching-owned-nfts)
    - [Fetching Created/Minted NFTs](#fetching-createdminted-nfts)
    - [Fetching Buy/Sell History](#fetching-buysell-history)
    - [Pagination](#pagination)
    - [Incremental Sync](#incremental-sync)
3. [objkt API](#objkt-api)
    - [GraphQL Basics](#graphql-basics)
    - [Useful Queries](#useful-queries)
4. [Data Points Available](#data-points-available)
5. ["2025 Wrapped" Query Examples](#2025-wrapped-query-examples)
6. [Best Practices & Lessons Learned](#best-practices--lessons-learned)

---

## API Overview

| API       | Type             | Best For                                             | Rate Limits           |
| --------- | ---------------- | ---------------------------------------------------- | --------------------- |
| **TZKT**  | REST + WebSocket | Raw blockchain data, transfers, balances, operations | Generous, ~10 req/sec |
| **objkt** | GraphQL          | Marketplace data, listings, sales, artist profiles   | Moderate              |

**Recommendation:** Use TZKT for on-chain data (ownership, transfers, mints). Use objkt for marketplace-specific data (listings, sales prices, collection stats).

---

## TZKT API

Base URL: `https://api.tzkt.io/v1`

### Authentication

**None required!** TZKT is free and open. No API key needed.

For high-volume usage, consider running your own TZKT instance or contacting them for enterprise support.

### Core Endpoints

| Endpoint              | Purpose                                     |
| --------------------- | ------------------------------------------- |
| `/tokens/balances`    | Current NFT holdings for an address         |
| `/tokens`             | Token metadata, filtered by creator         |
| `/tokens/transfers`   | Transfer history (buys, sells, gifts)       |
| `/operations`         | Raw blockchain operations                   |
| `/accounts/{address}` | Account info, last activity                 |
| `/head`               | Current blockchain head (for sync tracking) |

### Fetching Owned NFTs

Get all NFTs currently owned by a wallet:

```
GET /v1/tokens/balances
```

**Parameters:**

```
account={wallet_address}
token.standard=fa2
balance.gt=0
limit=1000
select=token.id,token.contract,token.tokenId,token.metadata,balance,lastLevel
```

**Example:**

```bash
curl "https://api.tzkt.io/v1/tokens/balances?account=tz1abc...&token.standard=fa2&balance.gt=0&limit=1000&select=token.id,token.contract,token.tokenId,token.metadata"
```

**Response:**

```json
[
    {
        "token": {
            "id": 12345678,
            "contract": { "address": "KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton" },
            "tokenId": "123456",
            "metadata": {
                "name": "My NFT",
                "description": "A cool NFT",
                "artifactUri": "ipfs://QmXxx...",
                "displayUri": "ipfs://QmYyy...",
                "thumbnailUri": "ipfs://QmZzz...",
                "formats": [{ "uri": "ipfs://QmXxx...", "mimeType": "image/png" }],
                "creators": ["tz1artist..."],
                "tags": ["art", "digital"]
            }
        },
        "balance": "1",
        "lastLevel": 3500000
    }
]
```

### Fetching Created/Minted NFTs

Get all NFTs minted by a wallet (even if sold):

```
GET /v1/tokens
```

**Parameters:**

```
firstMinter={wallet_address}
standard=fa2
limit=1000
select=id,contract,tokenId,metadata,firstMinter,firstLevel,firstTime
```

**Example:**

```bash
curl "https://api.tzkt.io/v1/tokens?firstMinter=tz1abc...&standard=fa2&limit=1000"
```

**Key Fields:**

-   `firstMinter` - The wallet that minted the token
-   `firstLevel` - Block level when minted
-   `firstTime` - Timestamp when minted

### Fetching Buy/Sell History

This is where it gets interesting for "Wrapped" analytics. Use the **transfers** endpoint:

```
GET /v1/tokens/transfers
```

**Get all transfers involving a wallet:**

```
anyof.from.to={wallet_address}
token.standard=fa2
limit=1000
sort.desc=timestamp
select=from,to,token,amount,transactionId,timestamp,level
```

**Decode transfer types:**

| From   | To     | Type                                  |
| ------ | ------ | ------------------------------------- |
| `null` | wallet | **Mint** (you created it)             |
| other  | wallet | **Receive** (bought, gifted, airdrop) |
| wallet | other  | **Send** (sold, gifted)               |
| wallet | `null` | **Burn**                              |

**Example - Get all transfers for 2024:**

```bash
curl "https://api.tzkt.io/v1/tokens/transfers?anyof.from.to=tz1abc...&token.standard=fa2&timestamp.ge=2024-01-01&timestamp.lt=2025-01-01&limit=10000"
```

**Determine Buy vs Gift:**

Transfers don't directly tell you if something was bought or gifted. To determine:

1. **Check the transaction** - Look at the operation to see if tez was exchanged
2. **Cross-reference with marketplace** - Use objkt API to see if there was a sale

**Get operation details for a transfer:**

```
GET /v1/operations/{transactionId}
```

Look for `amount` field (in mutez, divide by 1,000,000 for tez).

### Pagination

TZKT uses offset pagination with `lastId`:

```
# First page
GET /v1/tokens/balances?account=tz1...&limit=1000

# Next pages - use lastId from previous response
GET /v1/tokens/balances?account=tz1...&limit=1000&offset.cr=12345678
```

**Tip:** `offset.cr` (cursor) is more efficient than `offset` for large datasets.

### Incremental Sync

For live updates, track the blockchain level:

1. Get current head: `GET /v1/head`
2. Store `level` after each sync
3. On next sync, filter: `lastLevel.gt={stored_level}`

**Example:**

```bash
# Get balances changed since level 3500000
curl "https://api.tzkt.io/v1/tokens/balances?account=tz1...&lastLevel.gt=3500000"
```

---

## objkt API

GraphQL endpoint: `https://data.objkt.com/v3/graphql`

### GraphQL Basics

objkt uses GraphQL, which is more flexible but requires POST requests:

```bash
curl -X POST https://data.objkt.com/v3/graphql \
  -H "Content-Type: application/json" \
  -d '{"query": "{ ... }"}'
```

### Useful Queries

**Get creator's sales:**

```graphql
query GetCreatorSales($address: String!, $since: timestamptz!) {
    event(
        where: { creator_address: { _eq: $address }, event_type: { _eq: "sale" }, timestamp: { _gte: $since } }
        order_by: { timestamp: desc }
    ) {
        token {
            token_id
            name
            artifact_uri
            fa_contract
        }
        price
        buyer_address
        seller_address
        timestamp
        marketplace
    }
}
```

**Get collector's purchases:**

```graphql
query GetCollectorPurchases($address: String!, $since: timestamptz!) {
    event(
        where: { buyer_address: { _eq: $address }, event_type: { _eq: "sale" }, timestamp: { _gte: $since } }
        order_by: { timestamp: desc }
    ) {
        token {
            token_id
            name
            artifact_uri
            fa_contract
            creator {
                address
                alias
            }
        }
        price
        seller_address
        timestamp
        marketplace
    }
}
```

**Get collection stats:**

```graphql
query GetCollection($contract: String!) {
    fa(where: { contract: { _eq: $contract } }) {
        name
        floor_price
        volume_24h
        volume_total
        owners
        items
    }
}
```

**Variables:**

```json
{
    "address": "tz1abc...",
    "since": "2024-01-01T00:00:00Z"
}
```

---

## Data Points Available

### From TZKT (On-Chain)

| Data Point       | Endpoint                  | Notes                        |
| ---------------- | ------------------------- | ---------------------------- |
| Current holdings | `/tokens/balances`        | What you own right now       |
| Minted tokens    | `/tokens?firstMinter=`    | Everything you created       |
| Transfer history | `/tokens/transfers`       | All ins and outs             |
| Mint timestamp   | `/tokens` → `firstTime`   | When you created it          |
| Token metadata   | `/tokens` → `metadata`    | Name, description, IPFS URIs |
| Creator address  | `/tokens` → `firstMinter` | Who made it                  |
| Contract address | `/tokens` → `contract`    | Which collection/platform    |

### From objkt (Marketplace)

| Data Point       | Query                                       | Notes                    |
| ---------------- | ------------------------------------------- | ------------------------ |
| Sale price       | `event(event_type: "sale")`                 | In mutez                 |
| Buyer/Seller     | `event` → `buyer_address`, `seller_address` |                          |
| Listing price    | `listing`                                   | Current asks             |
| Offer amount     | `bid`                                       | Current bids             |
| Artist profile   | `holder(address)`                           | Bio, alias, social links |
| Collection stats | `fa`                                        | Floor, volume, holders   |
| Royalties        | `token` → `royalties`                       | Secondary sale %         |

### Derived Metrics (For "Wrapped")

Calculate these from raw data:

| Metric               | How to Calculate                               |
| -------------------- | ---------------------------------------------- |
| Total spent          | Sum of `price` where `buyer_address = wallet`  |
| Total earned         | Sum of `price` where `seller_address = wallet` |
| NFTs collected       | Count of incoming transfers (excluding mints)  |
| NFTs sold            | Count of outgoing transfers                    |
| NFTs minted          | Count of tokens where `firstMinter = wallet`   |
| Top artist collected | Group buys by creator, count                   |
| Biggest purchase     | Max price where buyer                          |
| Most profitable flip | Max(sell price - buy price) per token          |

---

## "2025 Wrapped" Query Examples

### 1. Get All 2025 Activity for a Wallet

**Step 1: Mints**

```bash
curl "https://api.tzkt.io/v1/tokens?firstMinter=tz1abc...&firstTime.ge=2025-01-01&firstTime.lt=2026-01-01&limit=10000"
```

**Step 2: All transfers**

```bash
curl "https://api.tzkt.io/v1/tokens/transfers?anyof.from.to=tz1abc...&timestamp.ge=2025-01-01&timestamp.lt=2026-01-01&limit=10000"
```

**Step 3: Sale details from objkt**

```graphql
query Wrapped2025($address: String!) {
    # Your sales (as creator)
    creator_sales: event(
        where: {
            creator_address: { _eq: $address }
            event_type: { _eq: "sale" }
            timestamp: { _gte: "2025-01-01" }
            timestamp: { _lt: "2026-01-01" }
        }
    ) {
        price
        timestamp
        token {
            name
        }
    }

    # Your purchases
    purchases: event(
        where: {
            buyer_address: { _eq: $address }
            event_type: { _eq: "sale" }
            timestamp: { _gte: "2025-01-01" }
            timestamp: { _lt: "2026-01-01" }
        }
    ) {
        price
        timestamp
        token {
            name
            creator {
                address
                alias
            }
        }
    }

    # Your secondary sales
    secondary_sales: event(
        where: {
            seller_address: { _eq: $address }
            creator_address: { _neq: $address }
            event_type: { _eq: "sale" }
            timestamp: { _gte: "2025-01-01" }
            timestamp: { _lt: "2026-01-01" }
        }
    ) {
        price
        timestamp
        token {
            name
        }
    }
}
```

### 2. Calculate Key Stats

```javascript
// Pseudo-code for "Wrapped" stats

const stats = {
    // Minting
    totalMinted: mints.length,
    firstMintDate: mints[0]?.firstTime,

    // Collecting
    totalCollected: purchases.length,
    totalSpent: purchases.reduce((sum, e) => sum + e.price, 0) / 1_000_000, // tez
    favoriteArtist: getMostFrequent(purchases.map((p) => p.token.creator.address)),
    biggestPurchase: Math.max(...purchases.map((p) => p.price)) / 1_000_000,

    // Selling
    totalSold: secondary_sales.length,
    totalEarned: secondary_sales.reduce((sum, e) => sum + e.price, 0) / 1_000_000,

    // Creator earnings (from your art)
    creatorEarnings: creator_sales.reduce((sum, e) => sum + e.price, 0) / 1_000_000,

    // Activity
    mostActiveMonth: getMonthWithMostActivity(allEvents),
    totalTransactions: mints.length + purchases.length + secondary_sales.length,
};
```

---

## Best Practices & Lessons Learned

### 1. Use `select` to Minimize Payload

Always specify exactly what fields you need:

```
# Bad - returns everything
/v1/tokens/balances?account=tz1...

# Good - only what you need
/v1/tokens/balances?account=tz1...&select=token.id,token.metadata.name,token.metadata.artifactUri
```

### 2. Handle Pagination Properly

Large wallets can have 10,000+ NFTs. Always paginate:

```javascript
async function fetchAllBalances(address) {
    const results = [];
    let lastId = 0;

    while (true) {
        const batch = await fetch(
            `https://api.tzkt.io/v1/tokens/balances?account=${address}&limit=1000&offset.cr=${lastId}`
        ).then((r) => r.json());

        if (batch.length === 0) break;
        results.push(...batch);
        lastId = batch[batch.length - 1].token.id;
    }

    return results;
}
```

### 3. Token Metadata Can Be Missing

Not all tokens have proper metadata. Always handle nulls:

```javascript
const name = token.metadata?.name || `Token #${token.tokenId}`;
const imageUri = token.metadata?.displayUri || token.metadata?.artifactUri || token.metadata?.thumbnailUri;
```

### 4. IPFS URIs Need Resolution

Metadata contains `ipfs://` URIs. Convert to HTTP gateway:

```javascript
function resolveIpfsUri(uri) {
    if (!uri) return null;
    if (uri.startsWith("ipfs://")) {
        return `https://ipfs.io/ipfs/${uri.slice(7)}`;
    }
    return uri;
}
```

### 5. Prices are in Mutez

1 tez = 1,000,000 mutez. Always convert:

```javascript
const tezPrice = mutezPrice / 1_000_000;
```

### 6. Rate Limiting

TZKT is generous but not unlimited:

```javascript
// Add delay between requests
async function fetchWithDelay(urls) {
    const results = [];
    for (const url of urls) {
        results.push(await fetch(url).then((r) => r.json()));
        await new Promise((r) => setTimeout(r, 100)); // 100ms delay
    }
    return results;
}
```

### 7. objkt vs TZKT for Sales Data

-   **TZKT transfers** show that tokens moved, but not the price
-   **objkt events** show actual sale prices and marketplace info

For "Wrapped", you need both:

1. TZKT for comprehensive transfer history
2. objkt for sale prices and marketplace context

### 8. Contract Addresses to Know

| Contract                               | Platform          |
| -------------------------------------- | ----------------- |
| `KT1RJ6PbjHpwc3M5rw5s2Nbmefwbuwbdxton` | hic et nunc (HEN) |
| `KT1KEa8z6vWXDJrVqtMrAeDVzsvxat3kHaCE` | fxhash            |
| `KT1U6EHmNxJTkvaWJ4ThczG4FSDaHC21ssvi` | objkt.com         |
| `KT1LjmAdYQCLBjwv4S2oFkEzyHVkomAf5MrW` | Versum            |
| `KT1EpGgjQs73QfFJs9z7m1Mxm5MTnpC2tqse` | Kalamint          |

### 9. Caching Strategy

For a "Wrapped" app:

1. **Cache heavily** - Historical data doesn't change
2. **Store raw responses** - You might want to recalculate stats
3. **Update only recent data** - Use incremental sync with `timestamp.gt`

### 10. Error Handling

Networks fail. Always retry:

```javascript
async function fetchWithRetry(url, retries = 3) {
    for (let i = 0; i < retries; i++) {
        try {
            const response = await fetch(url);
            if (!response.ok) throw new Error(`HTTP ${response.status}`);
            return response.json();
        } catch (error) {
            if (i === retries - 1) throw error;
            await new Promise((r) => setTimeout(r, 1000 * (i + 1))); // Exponential backoff
        }
    }
}
```

---

## Quick Reference

### TZKT Cheatsheet

```bash
# Current NFT holdings
/v1/tokens/balances?account={addr}&balance.gt=0

# Created NFTs
/v1/tokens?firstMinter={addr}

# Transfer history
/v1/tokens/transfers?anyof.from.to={addr}

# Date filtering
&timestamp.ge=2025-01-01&timestamp.lt=2026-01-01

# Pagination
&limit=1000&offset.cr={lastId}

# Field selection
&select=token.id,token.metadata.name
```

### objkt GraphQL Cheatsheet

```graphql
# Sales
event(where: { buyer_address: { _eq: $addr }, event_type: { _eq: "sale" } })

# Listings
listing(where: { seller_address: { _eq: $addr }, status: { _eq: "active" } })

# Token details
token(where: { fa_contract: { _eq: $contract }, token_id: { _eq: $id } })

# Artist profile
holder(where: { address: { _eq: $addr } })
```

---

## Further Reading

-   [TZKT API Docs](https://api.tzkt.io/)
-   [objkt API Docs](https://docs.objkt.com/)
-   [Tezos Token Standards (TZIP)](https://tzip.tezosagora.org/)
-   [FA2 Token Standard](https://gitlab.com/tezos/tzip/-/blob/master/proposals/tzip-12/tzip-12.md)
