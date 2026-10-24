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
    return (w >> c) | (w << (32 - c));
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

__device__ static void compress_pre(unsigned int state[16], const unsigned int cv[8],
                                    const unsigned char block[BLAKE3_BLOCK_LEN],
                                    unsigned long long counter, unsigned int block_len,
                                    unsigned int flags) {
    unsigned int block_words[16];
    for (int i = 0; i < 16; i++) {
        const unsigned char *p = block + 4 * i;
        block_words[i] = (unsigned int)p[0] | ((unsigned int)p[1] << 8) |
                         ((unsigned int)p[2] << 16) | ((unsigned int)p[3] << 24);
    }
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
    unsigned char block[BLAKE3_BLOCK_LEN];
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
    unsigned char block[BLAKE3_BLOCK_LEN];
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

// prefix: prev_hash_hex_ascii_lower || miner_address_utf8 (no nonce).
// Each thread appends nonce as 8-byte little-endian and Blake3-hashes.
// When prefix_len >= 64 (production tip hex), one thread compresses the first
// 64B block into shared CV; others only hash suffix||nonce — cuts ~½ compress work
// and avoids re-reading the tip bytes from shared mem per nonce.
extern "C" __global__ void poh_search_blake3(
    unsigned long long base,
    unsigned long long count,
    unsigned long long mod,
    const unsigned char* prefix,
    int prefix_len,
    unsigned long long* out_nonce)
{
    __shared__ unsigned char sprefix[960];
    __shared__ unsigned int scv[8];
    __shared__ int s_use_cv;
    if (prefix_len < 0 || prefix_len > 960 || mod == 0ULL) {
        return;
    }
    for (int i = (int)threadIdx.x; i < prefix_len; i += (int)blockDim.x) {
        sprefix[i] = prefix[i];
    }
    __syncthreads();

    if (threadIdx.x == 0) {
        s_use_cv = 0;
        if (prefix_len >= BLAKE3_BLOCK_LEN) {
            unsigned int cv[8];
            for (int i = 0; i < 8; i++) cv[i] = BLAKE3_IV[i];
            unsigned char block[BLAKE3_BLOCK_LEN];
            for (int i = 0; i < BLAKE3_BLOCK_LEN; i++) block[i] = sprefix[i];
            unsigned int state[16];
            compress_pre(state, cv, block, 0, BLAKE3_BLOCK_LEN, BLAKE3_CHUNK_START);
            for (int i = 0; i < 8; i++) scv[i] = state[i] ^ state[i + 8];
            s_use_cv = 1;
        }
    }
    __syncthreads();

    size_t tid = (size_t)blockIdx.x * (size_t)blockDim.x + (size_t)threadIdx.x;
    if (tid >= (size_t)count) {
        return;
    }
    unsigned long long n = base + (unsigned long long)tid;
    // Production tip(64 hex)+HMC-addr(~20)+nonce(8) ≈ 92B. Cap stack to keep occupancy.
    int plen = prefix_len;
    if (plen + 8 > 160) {
        return;
    }
    unsigned char digest[32];
    if (s_use_cv) {
        int rem = plen - BLAKE3_BLOCK_LEN;
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
        blake3_hash_from_cv(scv, tail, (size_t)rem + 8, digest);
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
        blake3_hash_one_chunk(msg, (size_t)plen + 8, digest);
    }
    unsigned long long work =
        (unsigned long long)digest[0] |
        ((unsigned long long)digest[1] << 8) |
        ((unsigned long long)digest[2] << 16) |
        ((unsigned long long)digest[3] << 24) |
        ((unsigned long long)digest[4] << 32) |
        ((unsigned long long)digest[5] << 40) |
        ((unsigned long long)digest[6] << 48) |
        ((unsigned long long)digest[7] << 56);
    if ((work % mod) != 0ULL) {
        return;
    }
    atomicMin(out_nonce, n);
}
