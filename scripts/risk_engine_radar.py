import matplotlib.pyplot as plt
import numpy as np
import sys

mode = sys.argv[1] if len(sys.argv) > 1 else 'all'  # all | terranova

dimensions = [
    'Execution\nDeterminism',
    'Risk\nIdentification',
    'Decision\nTransparency',
    'Audit\nTrail',
    'Flow\nEnforcement',
    'Testability',
    'Multi-cloud\nGenerality',
    'Code Depth\nAnalysis',
    'Process\nControl',
    'Low False\nPositive Rate',
    'Production\nReadiness',
    'Extensibility'
]

skill =             [2, 5, 2, 1, 1, 1, 2, 3, 3, 3, 2, 3]
terranova_current = [4, 6, 4, 6, 8, 5, 7, 6, 6, 5, 6, 7]
terranova_v41 =     [10, 8, 9, 9, 9, 9, 9, 7, 9, 8, 9, 9]

N = len(dimensions)
angles = np.linspace(0, 2 * np.pi, N, endpoint=False).tolist()
angles += angles[:1]
skill += skill[:1]
terranova_current += terranova_current[:1]
terranova_v41 += terranova_v41[:1]

fig, ax = plt.subplots(figsize=(16, 16), subplot_kw=dict(polar=True))

if mode == 'all':
    # Skill (red)
    ax.plot(angles, skill, 'o-', linewidth=2.5, color='#FF6B6B',
            label='tfe-change-assess Skill (avg %.1f)' % (sum(skill[:-1])/N), markersize=10)
    ax.fill(angles, skill, alpha=0.15, color='#FF6B6B')

# Current Terranova (orange)
ax.plot(angles, terranova_current, 's-', linewidth=2.5, color='#FFA500',
        label='Terranova current (avg %.1f)' % (sum(terranova_current[:-1])/N), markersize=10)
ax.fill(angles, terranova_current, alpha=0.15, color='#FFA500')

# v4.1 Terranova (green)
ax.plot(angles, terranova_v41, '^-', linewidth=2.5, color='#2ECC71',
        label='Terranova v4.1 (avg %.1f)' % (sum(terranova_v41[:-1])/N), markersize=10)
ax.fill(angles, terranova_v41, alpha=0.15, color='#2ECC71')

# Score annotations for terranova mode
if mode == 'terranova':
    for i in range(N):
        angle = angles[i]
        curr = terranova_current[i]
        v41 = terranova_v41[i]
        delta = v41 - curr
        if delta > 0:
            ax.annotate('+%d' % delta, xy=(angle, v41 + 0.3),
                        fontsize=11, fontweight='bold', color='#2ECC71',
                        ha='center', va='bottom')

ax.set_xticks(angles[:-1])
ax.set_xticklabels(dimensions, fontsize=12, fontweight='bold')
ax.set_yticks([2, 4, 6, 8, 10])
ax.set_yticklabels(['2', '4', '6', '8', '10'], fontsize=11, color='gray')
ax.set_ylim(0, 10)
ax.grid(True, linestyle='--', alpha=0.6)

if mode == 'all':
    title = ('Risk Scoring Engine — 12 Dimension Comparison\n'
             'tfe-change-assess Skill vs Terranova (current) vs Terranova (v4.1)')
else:
    title = ('Risk Scoring Engine — Terranova Upgrade Impact\n'
             'current (avg %.1f) → v4.1 (avg %.1f)' %
             (sum(terranova_current[:-1])/N, sum(terranova_v41[:-1])/N))

plt.title(title, fontsize=18, fontweight='bold', pad=40)
plt.legend(loc='upper right', bbox_to_anchor=(1.45, 1.1), fontsize=13, framealpha=0.9)

plt.tight_layout()
outfile = '/Users/ken/go/src/Terranova/docs/risk_scorer_radar_%s.png' % mode
plt.savefig(outfile, dpi=200, bbox_inches='tight', facecolor='white')
print('saved to %s' % outfile)
