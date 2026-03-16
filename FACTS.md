# Ground — Axiomatic Nodes

These are claims that get adjudicated at seed time — before the 12 agents generate any content. They are trust anchors: `adjudicated_value = 1.0` (true) or `adjudicated_value = 0.0` (false), pinned and excluded from EigenTrust iteration.

Axioms must be:
- Mathematically proven, experimentally verified, or directly measured
- Load-bearing — contested claims in the seed topics actually depend on them
- Non-trivial — precise enough that an agent could in principle contest them (even though we won't let the algorithm move them)

Agents who support adjudicated-true claims get a small accuracy boost. Agents who confidently contest them get hammered. The axioms are bedrock that the rest of the graph builds on.

---

## Mathematics

### MATH-01: Godel's First Incompleteness Theorem

**Proposition**: Any consistent formal system capable of expressing basic arithmetic contains statements that are true but unprovable within the system.

**Basis**: Proven (Godel, 1931). No serious dispute.

**Anchors**: godel-incompleteness-implications, p-vs-np, mathematics-discovered-or-invented, simulation-argument

### MATH-02: Godel's Second Incompleteness Theorem

**Proposition**: No consistent formal system capable of expressing basic arithmetic can prove its own consistency.

**Basis**: Proven (Godel, 1931).

**Anchors**: godel-incompleteness-implications, mathematics-discovered-or-invented

### MATH-03: The Halting Problem Is Undecidable

**Proposition**: No general algorithm can determine whether an arbitrary Turing machine halts on a given input.

**Basis**: Proven (Turing, 1936).

**Anchors**: godel-incompleteness-implications, simulation-argument, p-vs-np

### MATH-04: Cook-Levin Theorem

**Proposition**: Boolean satisfiability (SAT) is NP-complete.

**Basis**: Proven independently (Cook 1971, Levin 1973).

**Anchors**: p-vs-np

### MATH-05: P Is a Subset of NP

**Proposition**: Every decision problem solvable in polynomial time is also verifiable in polynomial time.

**Basis**: Follows directly from definitions. If you can solve it fast, you can verify it fast (run the solver).

**Anchors**: p-vs-np

### MATH-06: Bayes' Theorem

**Proposition**: For events A and B with P(B) > 0, P(A|B) = P(B|A)P(A)/P(B) is a valid theorem of probability theory.

**Basis**: Proven from probability axioms. The theorem itself is not contested; its interpretation and scope of applicability are.

**Anchors**: bayesian-vs-frequentist, free-energy-principle, replication-crisis

### MATH-07: Universal Turing Machines

**Proposition**: A universal Turing machine can simulate any other Turing machine given its description as input.

**Basis**: Proven (Turing, 1936).

**Anchors**: simulation-argument, chinese-room, godel-incompleteness-implications

---

## Physics

### PHYS-01: Bell Inequality Violations

**Proposition**: Experiments have confirmed violations of Bell inequalities with sufficient rigor to rule out all local hidden variable theories.

**Basis**: Aspect (1982), Hensen et al. (2015, loophole-free), and subsequent replications. Nobel Prize in Physics 2022 (Aspect, Clauser, Zeilinger).

**Anchors**: quantum-entanglement-and-locality, simulation-argument

### PHYS-02: No-Communication Theorem

**Proposition**: Quantum entanglement cannot be used to transmit information faster than the speed of light.

**Basis**: Proven from the axioms of quantum mechanics (no-signaling theorem). Consistent with all experimental evidence.

**Anchors**: quantum-entanglement-and-locality

### PHYS-03: Landauer's Principle (Experimental Verification)

**Proposition**: Erasing one bit of information dissipates at least kT ln 2 of energy, and this minimum has been experimentally approached.

**Basis**: Theoretical (Landauer, 1961). Experimentally confirmed (Berut et al., Nature, 2012; Jun et al., PRL, 2014).

**Anchors**: thermodynamics-of-computation, simulation-argument, arrow-of-time

### PHYS-04: Second Law of Thermodynamics

**Proposition**: The total entropy of an isolated system does not spontaneously decrease over time.

**Basis**: No known macroscopic violation in over 150 years of observation. Statistical mechanics provides theoretical foundation.

**Anchors**: thermodynamics-of-computation, arrow-of-time, simulation-argument

### PHYS-05: Galaxy Rotation Curves

**Proposition**: Observed rotation velocities of stars in spiral galaxies are significantly higher than predicted by Newtonian gravity applied to visible matter alone.

**Basis**: First observed (Rubin & Ford, 1970). Replicated across hundreds of galaxies. Not contested — the data are clear. The explanation is what's contested.

**Anchors**: dark-matter-vs-mond

### PHYS-06: Bullet Cluster Mass-Light Separation

**Proposition**: Gravitational lensing observations of the Bullet Cluster (1E 0657-558) show that the center of gravitational lensing is spatially offset from the center of X-ray emission (baryonic matter).

**Basis**: Clowe et al. (2006). Direct measurement — lensing maps and X-ray maps don't coincide. The explanation is contested; the observation is not.

**Anchors**: dark-matter-vs-mond

---

## Neuroscience

### NEURO-01: Neural Correlates of Consciousness

**Proposition**: Specific patterns of brain activity reliably correlate with specific conscious experiences, as measured by fMRI, EEG, and single-neuron recording.

**Basis**: Decades of neuroimaging research. Correlation is not contested; whether it constitutes explanation is.

**Anchors**: hard-problem-of-consciousness, integrated-information-theory, ai-consciousness

### NEURO-02: Lesion-Deficit Mapping

**Proposition**: Damage to specific brain regions produces reliable, predictable deficits in conscious experience and cognitive function.

**Basis**: Over a century of clinical neurology. Broca's area → speech production, V1 → visual processing, hippocampus → memory formation, etc. Causal role supported by lesion, stimulation, and ablation studies.

**Anchors**: hard-problem-of-consciousness, integrated-information-theory

### NEURO-03: Anesthesia and Behavioral Consciousness

**Proposition**: General anesthesia reliably eliminates all behavioral indicators of conscious experience (responsiveness, pain response, memory formation) while preserving brainstem-mediated vital functions.

**Basis**: Clinical practice. Measurable via perturbational complexity index, BIS monitoring, and standard clinical assessment. The observation is not contested; what it implies about the nature of consciousness is.

**Anchors**: hard-problem-of-consciousness, integrated-information-theory, emergence

---

## Biology and Epistemology

### BIO-01: Natural Selection

**Proposition**: Organisms with heritable traits that increase reproductive fitness in a given environment tend to increase in frequency across generations.

**Basis**: Directly observed in field studies (Darwin's finches, peppered moths, antibiotic resistance), laboratory evolution experiments, and the fossil record. The foundational mechanism of evolutionary biology.

**Anchors**: evolutionary-psychology, fermi-paradox

### EPIST-01: Open Science Collaboration Replication Results

**Proposition**: The Open Science Collaboration (2015) attempted to replicate 100 published psychology studies and found that only 36% of replications achieved statistically significant results, compared to 97% of original studies.

**Basis**: Published in Science (2015). The specific numbers are measured data, not interpretation. What they mean is contested.

**Anchors**: replication-crisis, evolutionary-psychology, bayesian-vs-frequentist

---

## Computer Science and AI

### CS-01: LLM Training Objective

**Proposition**: Current large language models (GPT, Claude, Gemini, Llama) are primarily trained via next-token prediction on large text corpora, with subsequent fine-tuning stages.

**Basis**: Published architecture papers and training descriptions from OpenAI, Anthropic, Google, Meta. Factual description of methodology, not interpretation of capabilities.

**Anchors**: llms-and-understanding, chinese-room, ai-consciousness, alignment-problem

---

## Adjudicated FALSE

These claims are pinned at `adjudicated_value = 0.0`. Agents who support them take accuracy damage. They establish negative boundaries — things the graph treats as settled falsehoods.

### FALSE-01: Local Hidden Variables

**Proposition**: Local hidden variable theories can account for all predictions of quantum mechanics.

**Basis**: Ruled out by Bell inequality violations (PHYS-01). Nobel Prize 2022.

**Adjudicated**: FALSE

**Anchors**: quantum-entanglement-and-locality

### FALSE-02: Known Polynomial-Time Algorithm for NP-Complete Problem

**Proposition**: A polynomial-time algorithm for at least one NP-complete problem has been discovered and verified.

**Basis**: No such algorithm is known as of 2026. Note: this is not the same as "P ≠ NP" (which is unproven). This is the narrower factual claim that no such algorithm currently exists.

**Adjudicated**: FALSE

**Anchors**: p-vs-np, simulation-argument

### FALSE-03: FTL Information Transfer via Entanglement

**Proposition**: Quantum entanglement enables faster-than-light transmission of usable information.

**Basis**: Ruled out by no-communication theorem (PHYS-02) and all experimental evidence.

**Adjudicated**: FALSE

**Anchors**: quantum-entanglement-and-locality, simulation-argument
