import { Component, Input, ViewChild } from '@angular/core';
import { ImageModalComponent } from '../image-modal/image-modal.component';
import { CommonModule } from '@angular/common';

@Component({
  selector: 'app-search-result',
  templateUrl: './search-result.component.html',
  styleUrls: ['./search-result.component.scss'],
  standalone: true,
  imports: [CommonModule, ImageModalComponent]
})
export class SearchResultComponent {
  @Input() result!: { imageUrl: string; mediaName: string; type: string };
  @ViewChild(ImageModalComponent) imageModal!: ImageModalComponent;
  isModalOpen: boolean = false;

  openImageModal() {
    this.isModalOpen = true;
  }

  closeImageModal() {
    this.isModalOpen = false;
  }
}
