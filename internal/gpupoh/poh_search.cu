// PoH search kernels (NVRTC).
// v1: eval(n)=n*7+13; record smallest n with eval % mod == 0.
// v3: Blake3(prev_ascii || miner_ascii || nonce_u64_le); work = LE uint64(digest[0:8]); work % mod == 0.
// Mirror: kernels/poh_search.cu
//
// Blake3 short-message path matches Go chain.PohWorkBlake3 / gpupoh.Blake3Search encoding.
// Host SearchPoH(formula>=3) currently uses Go Blake3 for bit-exact parity; this kernel is
// compiled for device search when wired by cuda_searcher (same message layout).

extern "C" __global__ void poh_search(
    unsigned long long base,
    unsigned long long count,
    unsigned long long mod,
    unsigned long long* out_nonce)
{
    size_t tid = (size_t)blockIdx.x * (size_t)blockDim.x + (size_t)threadIdx.x;
    if (tid >= (size_t)count) {
        return;
    }
    unsigned long long n = base + (unsigned long long)tid;
    unsigned long long v = n * 7ULL + 13ULL;
    if (mod == 0ULL) {
        return;
    }
    if ((v % mod) != 0ULL) {
        return;
    }
    atomicMin(out_nonce, n);
}

// --- Compact BLAKE3 (hash only, messages <= 1024 bytes) ---
// Based on BLAKE3 portable reference (CC0 / Apache-2.0).

#define BLAKE3_KEY_LEN 32
#define BLAKE3_OUT_LEN 32
#define BLAKE3_BLOCK_LEN 64
#define BLAKE3_CHUNK_LEN 1024
#define BLAKE3_CHUNK_START (1u << 0)
#define BLAKE3_CHUNK_END (1u << 1)
#define BLAKE3_ROOT (1u << 3)

__device__ __constant__ unsigned int BLAKE3_IV[8] = {
    0x6A09E667UL, 0xBB67AE85UL, 0x3C6EF372UL, 0xA54FF53AUL,
    0x510E527FUL, 0x9B05688CUL, 0x1F83D9ABUL, 0x5BE0CD19UL};

// Precomputed MSG schedule (indices into original block_words). Same as iteratively
// applying BLAKE3_MSG_PERMUTATION — avoids O(round) re-permute work each compress.
#define BLAKE3_ROUND(state, msg, i0, i1, i2, i3, i4, i5, i6, i7, i8, i9, i10, i11, i12, i13, i14, i15) \
    do {                                                                                            \
        g((state), 0, 4, 8, 12, (msg)[i0], (msg)[i1]);                                              \
        g((state), 1, 5, 9, 13, (msg)[i2], (msg)[i3]);                                              \
        g((state), 2, 6, 10, 14, (msg)[i4], (msg)[i5]);                                             \
        g((state), 3, 7, 11, 15, (msg)[i6], (msg)[i7]);                                             \
        g((state), 0, 5, 10, 15, (msg)[i8], (msg)[i9]);                                             \
        g((state), 1, 6, 11, 12, (msg)[i10], (msg)[i11]);                                           \
        g((state), 2, 7, 8, 13, (msg)[i12], (msg)[i13]);                                            \
        g((state), 3, 4, 9, 14, (msg)[i14], (msg)[i15]);                                            \
    } while (0)

__device__ static unsigned int rotr32(unsigned int w, unsigned int c) {
    // Prefer SHF (funnel shift) over SHR+SHL+OR when the toolchain emits it.
    return __funnelshift_r(w, w, c);
}

__device__ static void g(unsigned int *state, size_t a, size_t b, size_t c, size_t d,
                         unsigned int x, unsigned int y) {
    state[a] = state[a] + state[b] + x;
    state[d] = rotr32(state[d] ^ state[a], 16);
    state[c] = state[c] + state[d];
    state[b] = rotr32(state[b] ^ state[c], 12);
    state[a] = state[a] + state[b] + y;
    state[d] = rotr32(state[d] ^ state[a], 8);
    state[c] = state[c] + state[d];
    state[b] = rotr32(state[b] ^ state[c], 7);
}

// LE uint32 loads (GPU is little-endian; matches BLAKE3 portable byte packing).
// Prefer this over per-byte OR — fewer instructions, better ILP into compress.
__device__ static void load_block_words_le(unsigned int out[16],
                                          const unsigned char block[BLAKE3_BLOCK_LEN]) {
#pragma unroll
    for (int i = 0; i < 16; i++) {
        out[i] = ((const unsigned int *)(const void *)block)[i];
    }
}

__device__ static void compress_pre(unsigned int state[16], const unsigned int cv[8],
                                    const unsigned char block[BLAKE3_BLOCK_LEN],
                                    unsigned long long counter, unsigned int block_len,
                                    unsigned int flags) {
    unsigned int block_words[16];
    load_block_words_le(block_words, block);
    for (int i = 0; i < 8; i++) state[i] = cv[i];
    state[8] = BLAKE3_IV[0];
    state[9] = BLAKE3_IV[1];
    state[10] = BLAKE3_IV[2];
    state[11] = BLAKE3_IV[3];
    state[12] = (unsigned int)counter;
    state[13] = (unsigned int)(counter >> 32);
    state[14] = block_len;
    state[15] = flags;
    // 7 rounds, MSG schedule fully unrolled (constant indices for NVRTC).
    BLAKE3_ROUND(state, block_words, 0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15);
    BLAKE3_ROUND(state, block_words, 2, 6, 3, 10, 7, 0, 4, 13, 1, 11, 12, 5, 9, 14, 15, 8);
    BLAKE3_ROUND(state, block_words, 3, 4, 10, 12, 13, 2, 7, 14, 6, 5, 9, 0, 11, 15, 8, 1);
    BLAKE3_ROUND(state, block_words, 10, 7, 12, 9, 14, 3, 13, 15, 4, 0, 11, 2, 5, 8, 1, 6);
    BLAKE3_ROUND(state, block_words, 12, 13, 9, 11, 15, 10, 14, 8, 7, 2, 5, 3, 0, 1, 6, 4);
    BLAKE3_ROUND(state, block_words, 9, 14, 11, 5, 8, 12, 15, 1, 13, 3, 0, 10, 2, 6, 4, 7);
    BLAKE3_ROUND(state, block_words, 11, 15, 5, 0, 1, 9, 8, 6, 14, 10, 2, 12, 3, 4, 7, 13);
}

__device__ static void blake3_hash_one_chunk(const unsigned char *input, size_t input_len,
                                             unsigned char out[BLAKE3_OUT_LEN]) {
    unsigned int cv[8];
    for (int i = 0; i < 8; i++) cv[i] = BLAKE3_IV[i];
    __align__(4) unsigned char block[BLAKE3_BLOCK_LEN];
    size_t offset = 0;
    unsigned int flags = BLAKE3_CHUNK_START;
    while (input_len - offset > BLAKE3_BLOCK_LEN) {
        for (int i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = input[offset + i];
        unsigned int state[16];
        compress_pre(state, cv, block, 0, BLAKE3_BLOCK_LEN, flags);
        for (int i = 0; i < 8; i++) cv[i] = state[i] ^ state[i + 8];
        offset += BLAKE3_BLOCK_LEN;
        flags = 0;
    }
    size_t rem = input_len - offset;
    for (int i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = 0;
    for (size_t i = 0; i < rem; i++) block[i] = input[offset + i];
    unsigned int block_flags = flags | BLAKE3_CHUNK_END | BLAKE3_ROOT;
    unsigned int state[16];
    compress_pre(state, cv, block, 0, (unsigned int)rem, block_flags);
    for (int i = 0; i < 8; i++) {
        unsigned int w = state[i] ^ state[i + 8];
        out[4 * i + 0] = (unsigned char)(w);
        out[4 * i + 1] = (unsigned char)(w >> 8);
        out[4 * i + 2] = (unsigned char)(w >> 16);
        out[4 * i + 3] = (unsigned char)(w >> 24);
    }
}

// Continue hashing after a prior full block (flags already advanced past CHUNK_START).
__device__ static void blake3_hash_from_cv(const unsigned int cv0[8],
                                          const unsigned char *input, size_t input_len,
                                          unsigned char out[BLAKE3_OUT_LEN]) {
    unsigned int cv[8];
    for (int i = 0; i < 8; i++) cv[i] = cv0[i];
    __align__(4) unsigned char block[BLAKE3_BLOCK_LEN];
    size_t offset = 0;
    unsigned int flags = 0;
    while (input_len - offset > BLAKE3_BLOCK_LEN) {
        for (int i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = input[offset + i];
        unsigned int state[16];
        compress_pre(state, cv, block, 0, BLAKE3_BLOCK_LEN, flags);
        for (int i = 0; i < 8; i++) cv[i] = state[i] ^ state[i + 8];
        offset += BLAKE3_BLOCK_LEN;
        flags = 0;
    }
    size_t rem = input_len - offset;
    for (int i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = 0;
    for (size_t i = 0; i < rem; i++) block[i] = input[offset + i];
    unsigned int block_flags = flags | BLAKE3_CHUNK_END | BLAKE3_ROOT;
    unsigned int state[16];
    compress_pre(state, cv, block, 0, (unsigned int)rem, block_flags);
    for (int i = 0; i < 8; i++) {
        unsigned int w = state[i] ^ state[i + 8];
        out[4 * i + 0] = (unsigned char)(w);
        out[4 * i + 1] = (unsigned char)(w >> 8);
        out[4 * i + 2] = (unsigned char)(w >> 16);
        out[4 * i + 3] = (unsigned char)(w >> 24);
    }
}

// Hot path: single partial block from CV → LE uint64 work (digest[0:8] only).
__device__ static unsigned long long blake3_work64_from_cv_words(
    const unsigned int cv0[8], const unsigned int block_words[16], unsigned int block_len)
{
    unsigned int state[16];
    // compress_pre expects a byte block; rebuild aligned bytes from words (LE).
    __align__(4) unsigned char block[BLAKE3_BLOCK_LEN];
#pragma unroll
    for (int i = 0; i < 16; i++) {
        ((unsigned int *)(void *)block)[i] = block_words[i];
    }
    compress_pre(state, cv0, block, 0, block_len, BLAKE3_CHUNK_END | BLAKE3_ROOT);
    unsigned int w0 = state[0] ^ state[8];
    unsigned int w1 = state[1] ^ state[9];
    return (unsigned long long)w0 | ((unsigned long long)w1 << 32);
}

__device__ static void put_u64_le_bytes(unsigned int words[16], int byte_off,
                                       unsigned long long n) {
#pragma unroll
    for (int i = 0; i < 8; i++) {
        int b = byte_off + i;
        int wi = b >> 2;
        int sh = (b & 3) * 8;
        unsigned int mask = 0xFFu << sh;
        words[wi] = (words[wi] & ~mask) | ((unsigned int)((n >> (8 * i)) & 0xFFu) << sh);
    }
}

// Exact work % mod == 0 without 64-bit division (Lemire / Montgomery form).
__device__ static int work_divisible_mod(unsigned long long work, unsigned long long inv,
                                        unsigned long long lim, unsigned int tz) {
    if (tz > 0u) {
        if ((work & ((1ULL << tz) - 1ULL)) != 0ULL) {
            return 0;
        }
        work >>= tz;
    }
    return (work * inv) <= lim;
}

// Nonces hashed per thread (grid covers ceil(count/NPT) threads). NPT=4 was
// REVERT pre-MSG-unroll; NPT=2 re-tried after word-load for occupancy.
#ifndef BLAKE3_NONCES_PER_THREAD
#define BLAKE3_NONCES_PER_THREAD 2
#endif

// prefix: prev_hash_hex_ascii_lower || miner_address_utf8 (no nonce).
// Each thread appends nonce as 8-byte little-endian and Blake3-hashes.
// When prefix_len >= 64 (production tip hex), one thread compresses the first
// 64B block into shared CV; others only hash suffix||nonce — cuts ~½ compress work
// and avoids re-reading the tip bytes from shared mem per nonce.
// mod_inv / mod_lim / mod_tz: host prepareBlake3ModDiv(mod) — exact % without GPU udiv.
// share_mod==0 disables share harvest. Otherwise every nonce whose work is
// divisible by share_mod is appended to out_shares (capped by max_shares) via
// atomicAdd — same hashes as the block search, no separate CPU rescan.
extern "C" __global__ void poh_search_blake3(
    unsigned long long base,
    unsigned long long count,
    unsigned long long mod,
    unsigned long long mod_inv,
    unsigned long long mod_lim,
    unsigned int mod_tz,
    const unsigned char* prefix,
    int prefix_len,
    unsigned long long* out_nonce,
    unsigned long long share_mod,
    unsigned long long share_inv,
    unsigned long long share_lim,
    unsigned int share_tz,
    unsigned int* out_share_count,
    unsigned long long* out_shares,
    unsigned int max_shares)
{
    __shared__ unsigned char sprefix[960];
    __shared__ unsigned int scv[8];
    __shared__ unsigned int stail_words[16];
    __shared__ int s_use_cv;
    __shared__ int s_rem;
    if (prefix_len < 0 || prefix_len > 960 || mod == 0ULL) {
        return;
    }
    for (int i = (int)threadIdx.x; i < prefix_len; i += (int)blockDim.x) {
        sprefix[i] = prefix[i];
    }
    __syncthreads();

    if (threadIdx.x == 0) {
        s_use_cv = 0;
        s_rem = 0;
#pragma unroll
        for (int i = 0; i < 16; i++) stail_words[i] = 0;
        if (prefix_len >= BLAKE3_BLOCK_LEN) {
            unsigned int cv[8];
            for (int i = 0; i < 8; i++) cv[i] = BLAKE3_IV[i];
            __align__(4) unsigned char block[BLAKE3_BLOCK_LEN];
            for (int i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = sprefix[i];
            unsigned int state[16];
            compress_pre(state, cv, block, 0, BLAKE3_BLOCK_LEN, BLAKE3_CHUNK_START);
            for (int i = 0; i < 8; i++) scv[i] = state[i] ^ state[i + 8];
            s_use_cv = 1;
            int rem = prefix_len - BLAKE3_BLOCK_LEN;
            s_rem = rem;
            // Word path only when suffix||nonce fits one Blake3 block (rem+8 <= 64).
            // rem can be up to 88 (prefix 152); writing all rem bytes into
            // stail_words[16] OOB-corrupts s_use_cv/s_rem then IllegalAddress.
            if (rem + 8 <= BLAKE3_BLOCK_LEN) {
                for (int i = 0; i < rem; i++) {
                    int wi = i >> 2;
                    int sh = (i & 3) * 8;
                    stail_words[wi] |= ((unsigned int)sprefix[BLAKE3_BLOCK_LEN + i]) << sh;
                }
            }
        }
    }
    __syncthreads();

    size_t tid = (size_t)blockIdx.x * (size_t)blockDim.x + (size_t)threadIdx.x;
    size_t idx0 = tid * (size_t)BLAKE3_NONCES_PER_THREAD;
    if (idx0 >= (size_t)count) {
        return;
    }
    // Production tip(64 hex)+HMC-addr(~20)+nonce(8) ≈ 92B. Cap stack to keep occupancy.
    int plen = prefix_len;
    if (plen + 8 > 160) {
        return;
    }
#pragma unroll
    for (int k = 0; k < BLAKE3_NONCES_PER_THREAD; k++) {
        size_t idx = idx0 + (size_t)k;
        if (idx >= (size_t)count) {
            break;
        }
        unsigned long long n = base + (unsigned long long)idx;
        unsigned long long work;
        if (s_use_cv && s_rem + 8 <= BLAKE3_BLOCK_LEN) {
            unsigned int words[16];
#pragma unroll
            for (int i = 0; i < 16; i++) words[i] = stail_words[i];
            put_u64_le_bytes(words, s_rem, n);
            work = blake3_work64_from_cv_words(scv, words, (unsigned int)(s_rem + 8));
        } else if (s_use_cv) {
            int rem = s_rem;
            unsigned char tail[96];
            for (int i = 0; i < rem; i++) {
                tail[i] = sprefix[BLAKE3_BLOCK_LEN + i];
            }
            tail[rem + 0] = (unsigned char)(n);
            tail[rem + 1] = (unsigned char)(n >> 8);
            tail[rem + 2] = (unsigned char)(n >> 16);
            tail[rem + 3] = (unsigned char)(n >> 24);
            tail[rem + 4] = (unsigned char)(n >> 32);
            tail[rem + 5] = (unsigned char)(n >> 40);
            tail[rem + 6] = (unsigned char)(n >> 48);
            tail[rem + 7] = (unsigned char)(n >> 56);
            unsigned char digest[32];
            blake3_hash_from_cv(scv, tail, (size_t)rem + 8, digest);
            work =
                (unsigned long long)digest[0] |
                ((unsigned long long)digest[1] << 8) |
                ((unsigned long long)digest[2] << 16) |
                ((unsigned long long)digest[3] << 24) |
                ((unsigned long long)digest[4] << 32) |
                ((unsigned long long)digest[5] << 40) |
                ((unsigned long long)digest[6] << 48) |
                ((unsigned long long)digest[7] << 56);
        } else {
            unsigned char msg[160];
            for (int i = 0; i < plen; i++) {
                msg[i] = sprefix[i];
            }
            msg[plen + 0] = (unsigned char)(n);
            msg[plen + 1] = (unsigned char)(n >> 8);
            msg[plen + 2] = (unsigned char)(n >> 16);
            msg[plen + 3] = (unsigned char)(n >> 24);
            msg[plen + 4] = (unsigned char)(n >> 32);
            msg[plen + 5] = (unsigned char)(n >> 40);
            msg[plen + 6] = (unsigned char)(n >> 48);
            msg[plen + 7] = (unsigned char)(n >> 56);
            unsigned char digest[32];
            blake3_hash_one_chunk(msg, (size_t)plen + 8, digest);
            work =
                (unsigned long long)digest[0] |
                ((unsigned long long)digest[1] << 8) |
                ((unsigned long long)digest[2] << 16) |
                ((unsigned long long)digest[3] << 24) |
                ((unsigned long long)digest[4] << 32) |
                ((unsigned long long)digest[5] << 40) |
                ((unsigned long long)digest[6] << 48) |
                ((unsigned long long)digest[7] << 56);
        }
        if (share_mod != 0ULL && out_shares != 0 && out_share_count != 0 && max_shares > 0u) {
            if (work_divisible_mod(work, share_inv, share_lim, share_tz)) {
                unsigned int i = atomicAdd(out_share_count, 1u);
                if (i < max_shares) {
                    out_shares[i] = n;
                }
            }
        }
        if (work_divisible_mod(work, mod_inv, mod_lim, mod_tz)) {
            atomicMin(out_nonce, n);
        }
    }
}
