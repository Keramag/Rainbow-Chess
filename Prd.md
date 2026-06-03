# Rainbow-Chess #


## 1. Game Type
Chess variant with:
- standard chess rules  
- fixed starting piece positions  
- modified color assignment system (symmetry-based)  

---

## 2. Board Setup
- 8×8 standard chessboard  
- All pieces placed in standard chess starting squares  
- No positional randomness  

### Standard placement:
- Kings: e1 / e8  
- Queens: d1 / d8  
- Rooks: a1, h1 / a8, h8  
- Bishops: c1, f1 / c8, f8  
- Knights: b1, g1 / b8, g8  
- Pawns: rank 2 / rank 7  

---

## 3. Color Assignment Rule (Core Mechanism)

### Rule: Symmetric Color Mapping
Piece colors are assigned using board symmetry, not fixed ownership per side.

### Definition:
- vertical symmetry axis: file d–e center line  
- mirror function: `mirror(x) = 7 - x`

### Constraint:
- For every piece at square (x, y), its mirrored square (7 - x, y) must have the opposite color.
- mirrored positions must have opposite colors.

---

## 4. Turn System
- White always starts first  
- Alternating turns  
- Standard chess move rules  

---

## 5. Movement Rules
All standard chess movements apply:
- rook  
- bishop  
- queen  
- knight  
- king  
- pawn  

Additional rules:
- check rules fully enforced  
- no illegal moves allowed (king cannot remain in check)  

---

## 6. Pawn Rules
- Moves 1 square forward  
- Captures diagonally  
- Promotion only allowed to:
  - Knight  
  - Bishop  

Not allowed:
- queen promotion  
- rook promotion  
- any other promotion types  

---

## 7. Win Condition
- Checkmate = win  
- Stalemate = draw (optional but recommended)  

---

## 8. Game State Model (Agent Requirements)

Agent must track:
- board state (8×8 matrix)  
- piece type  
- piece color  
- current turn  
- legal moves list  
- king safety (`in_check` flag)  

---

## 9. Initialization Rules

Engine must:
- place pieces in standard positions  
- assign colors using symmetry rule  
- validate:
  - symmetry constraint is satisfied  
  - both kings exist  
  - no illegal starting check states (optional safety check)  

---

## 10. Core Constraint Summary
- No random placement  
- Only color is “structured-random via symmetry”  
- Strict mirrored color inversion across board center  
- Pawn promotion limited to Knight/Bishop  
- Fully legal chess rules otherwise  
