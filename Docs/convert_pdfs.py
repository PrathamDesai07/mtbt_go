import os
from PyPDF2 import PdfReader

def convert_pdfs_to_text(folder_path):
    """
    Convert all PDF files in the specified folder to text files.
    
    Args:
        folder_path: Path to the folder containing PDF files
    """
    # Check if folder exists
    if not os.path.exists(folder_path):
        print(f"Error: Folder '{folder_path}' does not exist.")
        return
    
    # Get all PDF files in the folder
    pdf_files = [f for f in os.listdir(folder_path) if f.lower().endswith('.pdf')]
    
    if not pdf_files:
        print(f"No PDF files found in '{folder_path}'")
        return
    
    print(f"Found {len(pdf_files)} PDF file(s) to convert...\n")
    
    # Process each PDF file
    for pdf_file in pdf_files:
        pdf_path = os.path.join(folder_path, pdf_file)
        txt_file = pdf_file.rsplit('.', 1)[0] + '.txt'
        txt_path = os.path.join(folder_path, txt_file)
        
        try:
            print(f"Converting: {pdf_file}")
            
            # Read PDF
            reader = PdfReader(pdf_path)
            
            # Extract text from all pages
            text_content = []
            for page_num, page in enumerate(reader.pages, 1):
                text = page.extract_text()
                text_content.append(f"--- Page {page_num} ---\n{text}\n")
            
            # Write to text file
            with open(txt_path, 'w', encoding='utf-8') as txt_f:
                txt_f.write('\n'.join(text_content))
            
            print(f"✓ Created: {txt_file} ({len(reader.pages)} pages)\n")
            
        except Exception as e:
            print(f"✗ Error converting {pdf_file}: {str(e)}\n")
    
    print("Conversion complete!")

if __name__ == "__main__":
    # Path to the Docs folder
    docs_folder = "/teamspace/studios/this_studio/mtbt_go/Docs"
    
    convert_pdfs_to_text(docs_folder)
